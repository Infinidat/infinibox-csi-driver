/*
Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/log"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

const (
	// MinVolumeSize : volume will be created with this size if requested volume size is less than this values
	MinVolumeSize = 1 * bytesofGiB

	bytesofKiB = 1024

	kiBytesofGiB = 1024 * 1024

	bytesofGiB = kiBytesofGiB * bytesofKiB

	// When you ask "why?": https://github.com/golang/go/issues/25539#issuecomment-394615058
	K8S_MOUNT_PERMS = "020000775" // setgid bit

	DEFAULT_FS_GROUP_CHANGE_POLICY = "Always" // we currently only support "Always", not "OnRootMisMatch"

	// shanked from K8s
	// rwMask   = os.FileMode(0660)
	// roMask   = os.FileMode(0440)
	// execMask = os.FileMode(0110)
)

var zlog = log.Get() // grab the logger for storage package use

// used to look up expected service for protocol
var protoToServiceMap = map[string]string{
	common.PROTOCOL_NFS:   common.NS_NFS_SVC,
	common.PROTOCOL_TREEQ: common.NS_NFS_SVC,
	common.PROTOCOL_ISCSI: common.NS_ISCSI_SVC,
}

func isMountedByListMethod(targetHostPath string) (bool, error) {
	// Use List() to search for mount matching targetHostPath
	// Each mount in the list has this example form:
	// {/dev/mapper/mpathn /host/var/lib/kubelet/pods/d2f8fcf0-f816-4008-b8fe-5d5f16c854d0/volumes/kubernetes.io~csi/csi-f581f6711d/mount xfs [rw seclabel relatime nouuid attr2 inode64 logbufs=8 logbsize=64k sunit=128 swidth=2048 noquota] 0 0}
	//
	// type MountPoint struct {
	//    Device string
	//    Path   string
	//    Type   string
	//    Opts   []string // Opts may contain sensitive mount options (like passwords) and MUST be treated as such (e|        .g. not logged).
	//    Freq   int
	//    Pass   int
	// }

	zlog.Debug().Msgf("Checking mount path using mounter's List() and searching with path '%s'", targetHostPath)
	mounter := mount.NewWithoutSystemd("")
	mountList, mountListErr := mounter.List()
	if mountListErr != nil {
		zlog.Err(mountListErr)
		return true, mountListErr
	}
	zlog.Trace().Msgf("Mount path list: %v", mountList)

	// Search list for targetHostPath
	isMountedByListMethod := false
	for i := range mountList {
		if mountList[i].Path == targetHostPath {
			isMountedByListMethod = true
			break
		}
	}
	zlog.Debug().Msgf("Path '%s' is mounted: %t", targetHostPath, isMountedByListMethod)
	return isMountedByListMethod, nil
}

func cleanupOldMountDirectory(targetHostPath string) error {
	zlog.Debug().Msgf("Cleaning up old mount directory at '%s'", targetHostPath)
	isMountEmpty, isMountEmptyErr := IsDirEmpty(targetHostPath)
	// Verify mount/ directory is empty. Fail if mount/ is not empty as that may be volume data.
	if isMountEmptyErr != nil {
		err := fmt.Errorf("failed IsDirEmpty() using targetHostPath '%s': %v", targetHostPath, isMountEmptyErr)
		zlog.Error().Msgf(err.Error())
		return err
	}
	if !isMountEmpty {
		err := fmt.Errorf("error: mount/ directory at targetHostPath '%s' is not empty and may contain volume data", targetHostPath)
		zlog.Error().Msgf(err.Error())
		return err
	}
	zlog.Trace().Msgf("verified that targetHostPath directory '%s', aka mount path, is empty of files", targetHostPath)

	// Clean up mount/
	if _, statErr := os.Stat(targetHostPath); os.IsNotExist(statErr) {
		zlog.Debug().Msgf("mount point targetHostPath '%s' already removed", targetHostPath)
	} else {
		zlog.Trace().Msgf("removing mount point targetHostPath '%s'", targetHostPath)
		if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
			err := fmt.Errorf("after unmounting, failed to Remove() path '%s': %v", targetHostPath, removeMountErr)
			zlog.Error().Msgf(err.Error())
			return err
		}
	}
	zlog.Debug().Msgf("Removed mount point targetHostPath '%s'", targetHostPath)

	csiHostPath := strings.TrimSuffix(targetHostPath, "/mount")
	volData := "vol_data.json"
	volDataPath := filepath.Join(csiHostPath, volData)

	// Clean up csi-NNNNNNN/vol_data.json file
	if _, statErr := os.Stat(volDataPath); os.IsNotExist(statErr) {
		zlog.Trace().Msgf("%s already removed from path '%s'", volData, csiHostPath)
	} else {
		zlog.Trace().Msgf("removing %s from path '%s'", volData, volDataPath)
		if err := os.Remove(volDataPath); err != nil {
			zlog.Warn().Msgf("after unmounting, failed to remove %s from path '%s': %v", volData, volDataPath, err)
		}
		zlog.Debug().Msgf("Successfully removed %s from path '%s'", volData, volDataPath)
	}

	// Clean up csi-NNNNNNN directory
	if _, statErr := os.Stat(csiHostPath); os.IsNotExist(statErr) {
		zlog.Debug().Msgf("CSI volume directory '%s' already removed", csiHostPath)
	} else {
		zlog.Debug().Msgf("Removing CSI volume directory '%s'", csiHostPath)
		if err := os.Remove(csiHostPath); err != nil {
			zlog.Error().Msgf("After unmounting, failed to remove CSI volume directory '%s': %v", csiHostPath, err)
		}
		zlog.Debug().Msgf("Successfully removed CSI volume directory'%s'", csiHostPath)
	}
	return nil
}

// Unmount using targetPath and cleanup directories and files.
func unmountAndCleanUp(targetPath string) (err error) {
	zlog.Debug().Msgf("Unmounting and cleaning up pathf for targetPath '%s'", targetPath)

	mounter := mount.NewWithoutSystemd("")
	targetHostPath := path.Join("/host", targetPath)

	zlog.Debug().Msgf("Unmounting targetPath '%s'", targetPath)
	if err := mounter.Unmount(targetPath); err != nil {
		zlog.Warn().Msgf("failed to unmount targetPath '%s' but rechecking: %v", targetPath, err)
	} else {
		zlog.Debug().Msgf("Successfully unmounted targetPath '%s'", targetPath)
	}

	isMounted, isMountedErr := isMountedByListMethod(targetHostPath)
	if isMountedErr != nil {
		err := fmt.Errorf("error: failed to check if targetHostPath '%s' is unmounted after unmounting %v", targetHostPath, isMountedErr)
		zlog.Error().Msgf(err.Error())
		return err
	}
	if isMounted {
		// TODO - Should include volume ID
		err := fmt.Errorf("error: volume remains mounted at targetHostPath '%s'", targetHostPath)
		zlog.Error().Msgf(err.Error())
		return err
	}
	zlog.Debug().Msgf("Verified that targetHostPath '%s' is not mounted", targetHostPath)

	// Check if targetHostPath exists
	if _, err := os.Stat(targetHostPath); os.IsNotExist(err) {
		zlog.Debug().Msgf("targetHostPath '%s' does not exist and does not need to be cleaned up", targetHostPath)
		return nil
	}

	// Check if targetHostPath is a directory or a file
	isADir, isADirError := IsDirectory(targetHostPath)
	if isADirError != nil {
		err := fmt.Errorf("failed to check if targetHostPath '%s' is a directory: %v", targetHostPath, isADirError)
		zlog.Error().Msgf(err.Error())
		return err
	}

	if isADir {
		zlog.Debug().Msgf("targetHostPath '%s' is a directory, not a file", targetHostPath)
		if err := cleanupOldMountDirectory(targetHostPath); err != nil {
			zlog.Err(err)
			return err
		}
		zlog.Debug().Msgf("Successfully cleaned up directory based targetHostPath '%s'", targetHostPath)
	} else {
		// TODO - Could check this is a file using IsDirectory().
		zlog.Debug().Msgf("targetHostPath '%s' is a file, not a directory", targetHostPath)
		if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
			err := fmt.Errorf("failed to Remove() path '%s': %v", targetHostPath, removeMountErr)
			zlog.Error().Msgf(err.Error())
			return err
		}
		zlog.Debug().Msgf("Successfully cleaned up file based targetHostPath '%s'", targetHostPath)
	}

	return nil
}

func validateStorageClassParameters(requiredStorageClassParams, optionalSCParameters map[string]string, providedStorageClassParams map[string]string, api api.Client) error {
	// Loop through and check required parameters only, consciously ignore parameters that aren't required
	badParamsMap := make(map[string]string)
	for param, required_regex := range requiredStorageClassParams {
		if param_value, ok := providedStorageClassParams[param]; ok {
			if matched, _ := regexp.MatchString(required_regex, param_value); !matched {
				badParamsMap[param] = "required input parameter " + param_value + " didn't match expected pattern " + required_regex
			}
		} else {
			badParamsMap[param] = "parameter required but not provided"
		}
	}

	scProtocol := providedStorageClassParams[common.SC_STORAGE_PROTOCOL]
	scNetSpace := strings.Split(providedStorageClassParams[common.SC_NETWORK_SPACE], ",") // get network_space(s) as array

	// validate network protocol / networkspace compatability
	if scProtocol != common.PROTOCOL_FC {
		if err := validateProtocolToNetworkSpace(scProtocol, scNetSpace, api); err != nil {
			zlog.Err(err)
			return err
		}
	}

	for param, required_regex := range optionalSCParameters {
		if param_value, ok := providedStorageClassParams[param]; ok {
			if matched, _ := regexp.MatchString(required_regex, param_value); !matched {
				badParamsMap[param] = "Optional input parameter " + param_value + " didn't match expected pattern " + required_regex
			}
		}
	}

	// validate optional uid and gid parameters
	gidProvided := providedStorageClassParams[common.SC_GID]
	uidProvided := providedStorageClassParams[common.SC_UID]
	unixPermissionsProvided := providedStorageClassParams[common.SC_UNIX_PERMISSIONS]

	if gidProvided != "" {
		gid_int, err := strconv.Atoi(gidProvided)
		if err != nil || gid_int < -1 {
			badParamsMap[common.SC_GID] = "Optional input parameter " + common.SC_GID + " appears to not be a valid integer"
		}
	}
	if uidProvided != "" {
		uid_int, err := strconv.Atoi(uidProvided)
		if err != nil || uid_int < -1 {
			badParamsMap[common.SC_UID] = "Optional input parameter " + common.SC_UID + " appears to not be a valid integer"
		}
	}

	if unixPermissionsProvided != "" {
		_, err := strconv.ParseUint(unixPermissionsProvided, 8, 32)
		if err != nil {
			badParamsMap[common.SC_UNIX_PERMISSIONS] = "Optional input parameter " + common.SC_UNIX_PERMISSIONS + " appears to not be a valid integer"
		}
	}

	provTypeProvided := providedStorageClassParams[common.SC_PROVISION_TYPE]
	if provTypeProvided != "" {
		p := strings.ToUpper(provTypeProvided)
		if p != common.SC_THICK_PROVISION_TYPE && p != common.SC_THIN_PROVISION_TYPE {
			badParamsMap[common.SC_PROVISION_TYPE] = fmt.Sprintf("Optional input parameter %s entered as %s is required to be THICK or THIN", common.SC_PROVISION_TYPE, p)
		}
	}

	ssdEnabledProvided := providedStorageClassParams[common.SC_SSD_ENABLED]
	if ssdEnabledProvided != "" {
		_, err := strconv.ParseBool(ssdEnabledProvided)
		if err != nil {
			badParamsMap[common.SC_SSD_ENABLED] = fmt.Sprintf("Optional input parameter %s entered as %s is required to be true or false", common.SC_SSD_ENABLED, ssdEnabledProvided)
		}
	}

	// optional and used only for NFS and TREEQ
	snapdirVisibleProvided := providedStorageClassParams[common.SC_SNAPDIR_VISIBLE]
	if snapdirVisibleProvided != "" {
		_, err := strconv.ParseBool(snapdirVisibleProvided)
		if err != nil {
			badParamsMap[common.SC_SNAPDIR_VISIBLE] = fmt.Sprintf("Optional input parameter %s entered as %s is required to be true or false", common.SC_SNAPDIR_VISIBLE, snapdirVisibleProvided)
		}
	}

	maxVolsProvided := providedStorageClassParams[common.SC_MAX_VOLS_PER_HOST]

	if maxVolsProvided != "" {
		maxVols_int, err := strconv.Atoi(maxVolsProvided)
		if err != nil || maxVols_int < -1 {
			badParamsMap[common.SC_MAX_VOLS_PER_HOST] = fmt.Sprintf("Optional input parameter %s appears to not be a valid integer, value entered was %s", common.SC_MAX_VOLS_PER_HOST, maxVolsProvided)
		}
	}

	if len(badParamsMap) > 0 {
		e := fmt.Errorf("invalid StorageClass parameters provided: %s", badParamsMap)
		zlog.Err(e)
		return e
	}

	// TODO validate uid, guid, unix_permissions globally since it pertains to nfs/treeq/fc
	// uid should be integer >= -1, if set to -1, then it means don't change
	// gid should be integer >= -1, if set to -1, then it means don't change
	// unix_permissions should be valid octal value

	// TODO refactor potential - each protocol would implement a function to isolate it's
	// particular SC validation logic
	if providedStorageClassParams[common.SC_STORAGE_PROTOCOL] == common.PROTOCOL_NFS || providedStorageClassParams[common.SC_STORAGE_PROTOCOL] == common.PROTOCOL_TREEQ {
		if providedStorageClassParams[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
			// the case when nfs_export_permissions is not set by a user in the SC
		} else {
			permissionsMapArray, err := getPermissionMaps(providedStorageClassParams[common.SC_NFS_EXPORT_PERMISSIONS])
			if err != nil {
				zlog.Err(err)
				return err
			}

			// validation for uid,gid,unix_permissions
			if providedStorageClassParams[common.SC_UID] != "" || providedStorageClassParams[common.SC_GID] != "" || providedStorageClassParams[common.SC_UNIX_PERMISSIONS] != "" {
				if len(permissionsMapArray) > 0 {
					noRootSquash := permissionsMapArray[0]["no_root_squash"]
					if noRootSquash == false {
						e := fmt.Errorf("error: uid, gid, or unix_permissions were set, but no_root_squash is false, this is not valid, no_root_squash is required to be true for uid,gid,unix_permissions to be applied")
						zlog.Err(e)
						return e
					}
				}
			}
		}
	}

	_, err := api.OneTimeValidation(providedStorageClassParams[common.SC_POOL_NAME], providedStorageClassParams[common.SC_NETWORK_SPACE])
	if err != nil {
		zlog.Err(err)
		return err
	}

	return nil
}

// validateProtocolToNetworkSpace - ensure specified protocol is valid for specified network space
func validateProtocolToNetworkSpace(protocol string, networkSpaces []string, api api.Client) error {

	if len(networkSpaces) == 0 {
		err := fmt.Errorf("no network spaces provided")
		zlog.Err(err)
		return err
	}

	for _, ns := range networkSpaces {
		zlog.Debug().Msgf("validating ns=%s protocol=%s", ns, protocol)
		nSpace, err := api.GetNetworkSpaceByName(ns)
		if err != nil {
			// api call throws error
			zlog.Err(err)
			return err
		}
		if len(nSpace.Service) == 0 {
			// handle empty result - nSpace doesn't exist
			e := fmt.Errorf("ibox not configured with specified network space: '%s' Service is empty", ns)
			zlog.Err(e)
			return e
		}
		if nSpace.Service != protoToServiceMap[protocol] {
			// handle invalid protocol/networkspace configuration
			e := fmt.Errorf("specified network space '%s' does not support %s protocol with %s service", ns, protocol, nSpace.Service)
			zlog.Err(e)
			return e
		}
		zlog.Debug().Msgf("Network space %s supports %s protocol with %s service", ns, protocol, nSpace.Service)
	}

	return nil // returns here if all network spaces pass validation for protocol.
}

func copyRequestParameters(parameters, out map[string]string) {
	for key, val := range parameters {
		if val != "" {
			out[key] = val
		}
	}
}

func validateVolumeID(str string) (volprotoconf api.VolumeProtocolConfig, err error) {
	volproto := strings.Split(str, "$$")
	if len(volproto) != 2 {
		return volprotoconf, errors.New("volume Id and other details not found")
	}
	volprotoconf.VolumeID = volproto[0]
	volprotoconf.StorageType = volproto[1]
	return volprotoconf, nil
}

func getPermissionMaps(permission string) ([]map[string]interface{}, error) {
	permissionFixed := strings.Replace(permission, "'", "\"", -1)
	var permissionsMapArray []map[string]interface{}
	err := json.Unmarshal([]byte(permissionFixed), &permissionsMapArray)
	if err != nil {
		zlog.Error().Msgf("invalid %s format %v raw [%s] fixed [%s]", common.SC_NFS_EXPORT_PERMISSIONS, err, permission, permissionFixed)
		return permissionsMapArray, err
	}

	for _, pass := range permissionsMapArray {
		no_root_squash_str, ok := pass["no_root_squash"].(string)
		if ok {
			rootsq, err := strconv.ParseBool(no_root_squash_str)
			if err != nil {
				zlog.Debug().Msgf("failed to cast no_root_squash value in export permission - setting default value 'true'")
				rootsq = true
			}
			pass["no_root_squash"] = rootsq
		}
	}
	return permissionsMapArray, nil
}

// IsDirEmpty Check if a directory is empty. Return an isEmpty boolean and an error.
func IsDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

// IsDirectory Determine if a file represented  by `path` is a directory or not.
func IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return fileInfo.IsDir(), err
}

type StorageHelper interface {
	SetVolumePermissions(req *csi.NodePublishVolumeRequest) (err error)
	ValidateNFSPortalIPAddress(ipAddress string) (err error)
	GetNFSMountOptions(req *csi.NodePublishVolumeRequest) ([]string, error)
	NodeExpandVolumeSize(req *csi.NodeExpandVolumeRequest) (err error)
}

type Service struct{}

func (n Service) NodeExpandVolumeSize(req *csi.NodeExpandVolumeRequest) (err error) {
	zlog.Debug().Msgf("NodeExpandVolumeSize req %+v\n", req)

	// we only care about fc and iscsi, nfs takes care of the expansion already
	// typical request contains stuff like this
	volproto := strings.Split(req.GetVolumeId(), "$$")
	if len(volproto) != 2 {
		return status.Error(codes.NotFound, "volume Id does not follow '<id>$$<proto>' pattern")
	}

	// we dont' care about NFS since it resizes are handled already by NFS itself
	if volproto[1] == common.PROTOCOL_NFS || volproto[1] == common.PROTOCOL_TREEQ {
		zlog.Debug().Msg("skipping nfs or treeq, not handled by NodeExpandVolume")
		return nil
	}

	// 1 - run mount | grep <volume_path> to find the multipath device name (e.g. /dev/mapper/mpathwi)

	command := fmt.Sprintf("mount | grep %s", req.GetVolumePath())
	out, err := execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error getting multipath device name %s - from %s \n", err.Error(), req.GetVolumePath())
		return err
	}

	output := strings.TrimSpace(string(out))
	outputParts := strings.Split(output, " ")
	multipathDevice := outputParts[0]

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	command = fmt.Sprintf("multipath -l %s | tail -n +4", multipathDevice)
	out, err = execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error getting multipath devices from output %s \n", err.Error())
		return err
	}

	output = strings.TrimSpace(string(out))
	zlog.Debug().Msgf("output is [%s]\n", output)
	outputParts = strings.Split(output, "\n")
	zlog.Debug().Msgf("lines %d\n", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := 0; i < len(outputParts); i++ {
		line := strings.TrimSpace(outputParts[i])
		lineParts := strings.Split(line, " ")
		if len(lineParts) < 3 {
			zlog.Error().Msgf("error getting multipath blockDevice from output %v", lineParts)
			continue
		}
		blockDevice := lineParts[2]
		zlog.Debug().Msgf("device is [%s]\n", blockDevice)
		rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
		command = fmt.Sprintf("echo 1 > %s", rescanPath)
		out, err := execScsi.Command(command, "")
		if err != nil {
			zlog.Error().Msgf("error writing rescan on multipath devices %s \n", err.Error())
			return err
		}
		zlog.Debug().Msgf("rescan output is [%s]\n", strings.TrimSpace(string(out)))
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		return fmt.Errorf("error getting mpathPart from %+v", mpathPart)
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	out, err = execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error multipathd resize map multipath devices %s \n", err.Error())
		return err
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]\n", strings.TrimSpace(string(out)))

	// 5 - run resize2fs /dev/mapper/mpathwi, this appears to work for both FC and iSCSI
	time.Sleep(time.Second * 5)
	command = fmt.Sprintf("resize2fs %s", multipathDevice)
	out, err = execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error resize2fs %s \n", err.Error())
		return err
	}
	zlog.Debug().Msgf("resize2fs output is [%s]\n", strings.TrimSpace(string(out)))

	return nil
}

func (n Service) GetNFSMountOptions(req *csi.NodePublishVolumeRequest) (mountOptions []string, err error) {
	// Get mount options from VolumeCapability - the standard way
	mountOptions = req.GetVolumeCapability().GetMount().GetMountFlags()
	if len(mountOptions) == 0 {
		for _, option := range strings.Split(StandardMountOptions, ",") {
			if option != "" {
				mountOptions = append(mountOptions, option)
			}
		}
	}

	mountOptions, err = updateNfsMountOptions(mountOptions, req)
	if err != nil {
		zlog.Error().Msgf("failed updateNfsMountOptions(): %s", err)
		return mountOptions, err
	}

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		mountOptions = append(mountOptions, "ro")
	}

	zlog.Debug().Msgf("nfs mount options are [%v]", mountOptions)

	return mountOptions, nil
}

func updateNfsMountOptions(mountOptions []string, req *csi.NodePublishVolumeRequest) ([]string, error) {
	// If vers set to anything but 3, fail.
	re := regexp.MustCompile(`(nfs){0,1}vers=([0-9]*)`)
	for _, opt := range mountOptions {
		matches := re.FindStringSubmatch(opt)
		if len(matches) > 0 {
			version := matches[2]
			if version != "3" {
				e := fmt.Errorf("nfs version mount option '%s' encountered, but only NFS version 3 is supported", opt)
				zlog.Err(e)
				return nil, e
			}
		}
	}

	// Force vers=3 to be in the mountOptions slice. IBoxes require NFS version 3.
	vers3InMountOptions := false
	for _, opt := range mountOptions {
		if opt == "vers=3" || opt == "nfsvers=3" {
			vers3InMountOptions = true
			break
		}
	}
	if !vers3InMountOptions {
		mountOptions = append(mountOptions, "vers=3")
	}

	// Add option hard if 'soft' not set explicitly.
	hardInMountOptions := false
	softInMountOptions := false
	for _, opt := range mountOptions {
		if opt == "hard" {
			hardInMountOptions = true
		}
		if opt == "soft" {
			softInMountOptions = true
		}
	}
	if !hardInMountOptions && !softInMountOptions {
		mountOptions = append(mountOptions, "hard")
	}

	// Support readonly mount option.
	if req.GetReadonly() {
		// TODO: ensure ro / rw behavior is correct, CSIC-343. eg what if user specifies "rw" as a mountOption?
		mountOptions = append(mountOptions, "ro")
	}

	// TODO: remove duplicates from this list

	return mountOptions, nil
}

func (n Service) SetVolumePermissions(req *csi.NodePublishVolumeRequest) (err error) {
	snapDir := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
	var snapdirVisible bool
	if snapDir != "" {
		snapdirVisible, err = strconv.ParseBool(snapDir)
		if err != nil {
			zlog.Err(err)
			return err
		}
	}
	targetPath := req.GetTargetPath()      // this is the path on the host node
	hostTargetPath := "/host" + targetPath // this is the path inside the csi container

	fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()
	fsGroupChangePolicy := DEFAULT_FS_GROUP_CHANGE_POLICY

	fsGroupIsSet := (fsGroup != "")

	zlog.Debug().Msgf("StorageHelper fsGroup: %s", fsGroup)

	// Chown
	var uid_int, gid_int int

	if !fsGroupIsSet { // use storageclass for volume mount parameters.

		tmp := req.GetVolumeContext()[common.SC_UID] // Returns an empty string if key not found
		if tmp == "" {
			uid_int = -1 // -1 means to not change the value
		} else {
			uid_int, err = strconv.Atoi(tmp)
			if err != nil || uid_int < -1 {
				e := fmt.Errorf("storage class specifies an invalid volume UID with value [%d]: %s", uid_int, err)
				zlog.Err(e)
				return e
			}
		}

		tmp = req.GetVolumeContext()[common.SC_GID]
		if tmp == "" {
			gid_int = -1 // -1 means to not change the value
		} else {
			gid_int, err = strconv.Atoi(tmp)
			if err != nil || gid_int < -1 {
				e := fmt.Errorf("storage class specifies an invalid volume GID with value [%d]: %s", gid_int, err)
				zlog.Err(e)
				return e
			}
		}

	} else { // use the fsGroup spec from the pod.

		gid_int, err = strconv.Atoi(fsGroup)
		if err != nil || gid_int < -1 {
			e := fmt.Errorf("storage class specifies an invalid volume GID with value [%d]: %s", gid_int, err)
			zlog.Err(e)
			return e
		}
		uid_int = -1 // indicate no change to uid since fsgroup being set.
	}

	zlog.Debug().Msgf("chown mount %s uid=%d gid=%d", hostTargetPath, uid_int, gid_int)
	if fsGroupIsSet {
		zlog.Debug().Msgf("for fsgroup, performing recursive chown")
		err = ChownR(hostTargetPath, uid_int, gid_int, fsGroupIsSet, fsGroupChangePolicy, snapdirVisible) // recursively set the path.
	} else {
		err = os.Chown(hostTargetPath, uid_int, gid_int)
	}

	if err != nil {
		e := fmt.Errorf("failed to chown path '%s': %v", hostTargetPath, err)
		zlog.Err(e)
		return status.Errorf(codes.Internal, e.Error())
	}

	var unixPermissions string
	// If fsGroupIsSet, we need to set perms on the mount with setgid bit.
	if fsGroupIsSet {
		unixPermissions = K8S_MOUNT_PERMS
	} else {
		unixPermissions = req.GetVolumeContext()[common.SC_UNIX_PERMISSIONS] // Returns an empty string if key not found
	}

	if unixPermissions != "" {
		tempVal, err := strconv.ParseUint(unixPermissions, 8, 32)
		if err != nil {
			e := fmt.Errorf("failed to convert unix_permissions '%s' error: %s", unixPermissions, err.Error())
			zlog.Err(e)
			return status.Errorf(codes.Internal, e.Error())
		}
		mode := uint(tempVal)
		err = os.Chmod(hostTargetPath, os.FileMode(mode))
		if err != nil {
			e := fmt.Errorf("failed to chmod path '%s' with perms %s: error: %v", hostTargetPath, unixPermissions, err)
			zlog.Err(e)
			return status.Errorf(codes.Internal, e.Error())
		}
		zlog.Debug().Msgf("chmod mount %s perms=%s", hostTargetPath, unixPermissions)
	}

	// print out the target permissions
	logPermissions("", filepath.Dir(hostTargetPath))

	return nil
}

// recursively chowns a root path
func ChownR(path string, uid int, gid int, fsGroupIsSet bool, fsGroupChangePolicy string, snapdirVisible bool) error {

	// this will exit early if there is a reason to not chown the files. Since we currently only support
	// "Always" for fsGroupChangePolicy, this block will not run, and files will always be chowned.
	// keeping since this was a pain to figure out. See
	// https://github.com/kubernetes/kubernetes/blob/8a62859e515889f07e3e3be6a1080413f17cf2c3/pkg/volume/volume_linux.go#L146
	if fsGroupIsSet && fsGroupChangePolicy != DEFAULT_FS_GROUP_CHANGE_POLICY {
		// note: if fsGroupIsSet, gid will have the fsGroup value.
		fsInfo, err := os.Stat(path)
		if err != nil {
			zlog.Error().Msgf("performing recursive ownership change on %s because reading permissions of root volume failed: %v", path, err)
			return nil
		}
		stat, ok := fsInfo.Sys().(*syscall.Stat_t)
		if !ok || stat == nil {
			zlog.Error().Msgf("performing recursive ownership change on %s because reading permissions of root volume failed", path)
			return nil
		}
		zlog.Debug().Msgf("Path: %s, volume gid %d , fsGroup: %d", path, stat.Gid, gid)
		// nothing to change if they match
		if int(stat.Gid) == gid {
			return nil
		}
		zlog.Debug().Msgf("expected group ownership of volume %s did not match with: %d", path, stat.Gid)

	}

	err := filepath.WalkDir(path,
		func(path string, d fs.DirEntry, err error) error {

			if err == nil {
				zlog.Debug().Msgf("Chown: %s with uid: %d and gid: %d", path, uid, gid)

				// handle the case on .snapshot hidden directories because they are readonly created by the ibox
				if snapdirVisible && d.Name() == ".snapshot" {
					zlog.Warn().Msgf("Chown: skipping chown on %s because snapdir_visible is true", d.Name())
					return filepath.SkipDir
				}

				// handle the broken symlink case, skip chown on broken symlinks
				if d.Type()&os.ModeSymlink != 0 {
					zlog.Warn().Msgf("Chown: we have a symlink %s!", path)
					_, e := os.ReadFile(path)
					if e != nil {
						zlog.Warn().Msgf("Chown: error reading link, assuming its a broken link %s, skipping chown on it", e.Error())
						return nil
					}
					zlog.Warn().Msgf("Chown: link is good %s", path)
				}

				err = os.Chown(path, uid, gid)
			}
			return err
		})

	return err
}

func logPermissions(note, hostTargetPath string) {
	// print out the target permissions
	cmd := exec.Command("ls", "-l", hostTargetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		zlog.Error().Msgf("error in doing ls command on %s error is  %s\n", hostTargetPath, err.Error())
	}
	zlog.Debug().Msgf("%s \nmount point permissions on %s ... %s", note, hostTargetPath, string(output))
}

func nfsSanityCheck(req *csi.CreateVolumeRequest, requiredParams map[string]string, optionalParams map[string]string, suppliedParams map[string]string, api api.Client) (capacity int64, err error) {

	err = validateStorageClassParameters(requiredParams, optionalParams, suppliedParams, api)
	if err != nil {
		zlog.Err(err)
		return capacity, status.Error(codes.InvalidArgument, err.Error())
	}

	capacity = int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("volume Minimum capacity should be greater 1 GB")
	}

	useChap := suppliedParams[common.SC_USE_CHAP]
	if useChap != "" {
		zlog.Warn().Msgf("useCHAP is not a valid storage class parameter for nfs or nfs-treeq")
	}

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("block access requested for %s PV %s", suppliedParams[common.SC_STORAGE_PROTOCOL], req.GetName())
			zlog.Err(e)
			return capacity, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	return capacity, err
}

// validateSnapshotLockingParameter validates an input lock_expires parameter string and returns
// the time in Unix Milliseconds or an error if the validation fails
func validateSnapshotLockingParameter(input string) (timeInUnixMilli int64, err error) {

	parts := strings.Split(input, " ")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid format of lock_expires_at parameter, should only have 2 values (int string)")
	}

	// we except the 1st part of the parameter to be an integer
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid format of lock_expires_at count, should be in the format of an integer")
	}

	if count < 1 {
		return 0, fmt.Errorf("invalid lock_expires_at count, should be greater than 0")
	}

	var years, months, days int

	var futureTime time.Time
	nowTime := time.Now()

	// input will look like '1 Hours', '1 Days', '1 Weeks', '1 Months', '1 Years'
	// this function converts an input value into a numerical value representing
	// a date in the future from the current time

	switch parts[1] {
	case "Hours":
		var futureDuration time.Duration
		futureDuration, err = time.ParseDuration(fmt.Sprintf("%dh", count))
		if err != nil {
			return 0, fmt.Errorf("invalid lock_expires_at, parse duration error %s", err.Error())
		}
		futureTime = nowTime.Add(futureDuration)
	case "Days":
		days = count
		futureTime = nowTime.AddDate(years, months, days)
	case "Weeks":
		hoursInWeeks := 168 * count
		var futureDuration time.Duration
		futureDuration, err = time.ParseDuration(fmt.Sprintf("%dh", hoursInWeeks))
		if err != nil {
			return 0, fmt.Errorf("invalid lock_expires_at, parse duration error %s", err.Error())
		}
		futureTime = nowTime.Add(futureDuration)
	case "Months":
		months = count
		futureTime = nowTime.AddDate(years, months, days)
	case "Years":
		years = count
		futureTime = nowTime.AddDate(years, months, days)
	default:
		return 0, fmt.Errorf("invalid format of lock_expires_at frequency, should be either Days, Hours, Weeks, Months, Years")
	}

	return futureTime.UnixMilli(), nil
}

func (n Service) ValidateNFSPortalIPAddress(ip string) (err error) {
	start := time.Now()

	const nfsPort = "2049"
	nfsAddress := fmt.Sprintf("%s:%s", ip, nfsPort)
	_, err = net.Dial("tcp", nfsAddress)
	elapsed := time.Since(start)

	if err != nil {
		zlog.Error().Msgf("error dialing NFS network space portal IP address %s - %s time: %s", nfsAddress, err.Error(), elapsed)
		return err
	}
	zlog.Debug().Msgf("NFS network space portal IP address %s is reachable, time: %s", nfsAddress, elapsed)
	return nil
}
