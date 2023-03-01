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
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"k8s.io/mount-utils"
)

const (
	// MinVolumeSize : volume will be created with this size if requested volume size is less than this values
	MinVolumeSize = 1 * bytesofGiB

	bytesofKiB = 1024

	kiBytesofGiB = 1024 * 1024

	bytesofGiB = kiBytesofGiB * bytesofKiB
)

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

	klog.V(4).Infof("Checking mount path using mounter's List() and searching with path '%s'", targetHostPath)
	mounter := mount.New("")
	mountList, mountListErr := mounter.List()
	if mountListErr != nil {
		klog.Error(mountListErr)
		return true, mountListErr
	}
	klog.V(5).Infof("Mount path list: %v", mountList)

	// Search list for targetHostPath
	isMountedByListMethod := false
	for i := range mountList {
		if mountList[i].Path == targetHostPath {
			isMountedByListMethod = true
			break
		}
	}
	klog.V(4).Infof("Path '%s' is mounted: %t", targetHostPath, isMountedByListMethod)
	return isMountedByListMethod, nil
}

func cleanupOldMountDirectory(targetHostPath string) error {
	klog.V(4).Infof("Cleaning up old mount directory at '%s'", targetHostPath)
	isMountEmpty, isMountEmptyErr := IsDirEmpty(targetHostPath)
	// Verify mount/ directory is empty. Fail if mount/ is not empty as that may be volume data.
	if isMountEmptyErr != nil {
		err := fmt.Errorf("failed IsDirEmpty() using targetHostPath '%s': %v", targetHostPath, isMountEmptyErr)
		klog.Errorf(err.Error())
		return err
	}
	if !isMountEmpty {
		err := fmt.Errorf("error: mount/ directory at targetHostPath '%s' is not empty and may contain volume data", targetHostPath)
		klog.Errorf(err.Error())
		return err
	}
	klog.V(4).Infof("verified that targetHostPath directory '%s', aka mount path, is empty of files", targetHostPath)

	// Clean up mount/
	if _, statErr := os.Stat(targetHostPath); os.IsNotExist(statErr) {
		klog.V(4).Infof("mount point targetHostPath '%s' already removed", targetHostPath)
	} else {
		klog.V(4).Infof("removing mount point targetHostPath '%s'", targetHostPath)
		if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
			err := fmt.Errorf("after unmounting, failed to Remove() path '%s': %v", targetHostPath, removeMountErr)
			klog.Errorf(err.Error())
			return err
		}
	}
	klog.V(4).Infof("Removed mount point targetHostPath '%s'", targetHostPath)

	csiHostPath := strings.TrimSuffix(targetHostPath, "/mount")
	volData := "vol_data.json"
	volDataPath := filepath.Join(csiHostPath, volData)

	// Clean up csi-NNNNNNN/vol_data.json file
	if _, statErr := os.Stat(volDataPath); os.IsNotExist(statErr) {
		klog.V(4).Infof("%s already removed from path '%s'", volData, csiHostPath)
	} else {
		klog.V(4).Infof("removing %s from path '%s'", volData, volDataPath)
		if err := os.Remove(volDataPath); err != nil {
			klog.Warningf("after unmounting, failed to remove %s from path '%s': %v", volData, volDataPath, err)
		}
		klog.V(4).Infof("Successfully removed %s from path '%s'", volData, volDataPath)
	}

	// Clean up csi-NNNNNNN directory
	if _, statErr := os.Stat(csiHostPath); os.IsNotExist(statErr) {
		klog.V(4).Infof("CSI volume directory '%s' already removed", csiHostPath)
	} else {
		klog.V(4).Infof("Removing CSI volume directory '%s'", csiHostPath)
		if err := os.Remove(csiHostPath); err != nil {
			klog.Errorf("After unmounting, failed to remove CSI volume directory '%s': %v", csiHostPath, err)
		}
		klog.V(4).Infof("Successfully removed CSI volume directory'%s'", csiHostPath)
	}
	return nil
}

// Unmount using targetPath and cleanup directories and files.
func unmountAndCleanUp(targetPath string) (err error) {
	klog.V(4).Infof("Unmounting and cleaning up pathf for targetPath '%s'", targetPath)
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("recovered from unmountAndCleanUp  " + fmt.Sprint(res))
		}
	}()

	mounter := mount.New("")
	targetHostPath := path.Join("/host", targetPath)

	klog.V(4).Infof("Unmounting targetPath '%s'", targetPath)
	if err := mounter.Unmount(targetPath); err != nil {
		klog.Warningf("failed to unmount targetPath '%s' but rechecking: %v", targetPath, err)
	} else {
		klog.V(4).Infof("Successfully unmounted targetPath '%s'", targetPath)
	}

	isMounted, isMountedErr := isMountedByListMethod(targetHostPath)
	if isMountedErr != nil {
		err := fmt.Errorf("error: failed to check if targetHostPath '%s' is unmounted after unmounting %v", targetHostPath, isMountedErr)
		klog.Errorf(err.Error())
		return err
	}
	if isMounted {
		// TODO - Should include volume ID
		err := fmt.Errorf("error: volume remains mounted at targetHostPath '%s'", targetHostPath)
		klog.Errorf(err.Error())
		return err
	}
	klog.V(4).Infof("Verified that targetHostPath '%s' is not mounted", targetHostPath)

	// Check if targetHostPath exists
	if _, err := os.Stat(targetHostPath); os.IsNotExist(err) {
		klog.V(4).Infof("targetHostPath '%s' does not exist and does not need to be cleaned up", targetHostPath)
		return nil
	}

	// Check if targetHostPath is a directory or a file
	isADir, isADirError := IsDirectory(targetHostPath)
	if isADirError != nil {
		err := fmt.Errorf("failed to check if targetHostPath '%s' is a directory: %v", targetHostPath, isADirError)
		klog.Errorf(err.Error())
		return err
	}

	if isADir {
		klog.V(4).Infof("targetHostPath '%s' is a directory, not a file", targetHostPath)
		if err := cleanupOldMountDirectory(targetHostPath); err != nil {
			klog.Error(err)
			return err
		}
		klog.V(4).Infof("Successfully cleaned up directory based targetHostPath '%s'", targetHostPath)
	} else {
		// TODO - Could check this is a file using IsDirectory().
		klog.V(4).Infof("targetHostPath '%s' is a file, not a directory", targetHostPath)
		if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
			err := fmt.Errorf("failed to Remove() path '%s': %v", targetHostPath, removeMountErr)
			klog.Errorf(err.Error())
			return err
		}
		klog.V(4).Infof("Successfully cleaned up file based targetHostPath '%s'", targetHostPath)
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
			klog.Error(err)
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

	if len(badParamsMap) > 0 {
		e := fmt.Errorf("invalid StorageClass parameters provided: %s", badParamsMap)
		klog.Error(e)
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
				klog.Error(err)
				return err
			}

			// validation for uid,gid,unix_permissions
			if providedStorageClassParams[common.SC_UID] != "" || providedStorageClassParams[common.SC_GID] != "" || providedStorageClassParams[common.SC_UNIX_PERMISSIONS] != "" {
				if len(permissionsMapArray) > 0 {
					noRootSquash := permissionsMapArray[0]["no_root_squash"]
					if noRootSquash == false {
						e := fmt.Errorf("error: uid, gid, or unix_permissions were set, but no_root_squash is false, this is not valid, no_root_squash is required to be true for uid,gid,unix_permissions to be applied")
						klog.Error(e)
						return e
					}
				}
			}
		}
	}

	return nil
}

// validateProtocolToNetworkSpace - ensure specified protocol is valid for specified network space
func validateProtocolToNetworkSpace(protocol string, networkSpaces []string, api api.Client) error {

	if len(networkSpaces) == 0 {
		err := fmt.Errorf("no network spaces provided")
		klog.Error(err)
		return err
	}

	for _, ns := range networkSpaces {
		if nSpace, err := api.GetNetworkSpaceByName(ns); err != nil {
			// api call throws error
			klog.Error(err)
			return err
		} else if len(nSpace.Service) == 0 {
			// handle empty result - nSpace doesn't exist
			e := fmt.Errorf("ibox not configured with specified network space: '%s'", ns)
			klog.Error(e)
			return e
		} else if nSpace.Service != protoToServiceMap[protocol] {
			// handle invalid protocol/networkspace configuration
			e := fmt.Errorf("specified network space '%s' does not support %s protocol with %s service", ns, protocol, nSpace.Service)
			klog.Error(e)
			return e
		} else {
			klog.Infof("Network space %s supports %s protocol with %s service", ns, protocol, nSpace.Service)
		}
	}

	return nil // returns here if all network spaces pass validation for protocol.
}

func copyRequestParameters(parameters, out map[string]string) {
	for key, val := range parameters {
		if val != "" {
			out[key] = val
			klog.V(4).Infof("%s: %s", key, val)
		} else {
			klog.V(4).Infof("%s: empty", key)
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
		klog.Errorf("invalid %s format %v raw [%s] fixed [%s]", common.SC_NFS_EXPORT_PERMISSIONS, err, permission, permissionFixed)
		return permissionsMapArray, err
	}

	for _, pass := range permissionsMapArray {
		no_root_squash_str, ok := pass["no_root_squash"].(string)
		if ok {
			rootsq, err := strconv.ParseBool(no_root_squash_str)
			if err != nil {
				klog.V(4).Infof("failed to cast no_root_squash value in export permission - setting default value 'true'")
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
	GetNFSMountOptions(req *csi.NodePublishVolumeRequest) ([]string, error)
}

type Service struct{}

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
		klog.Errorf("failed updateNfsMountOptions(): %s", err)
		return mountOptions, err
	}

	if req.GetReadonly() {
		// TODO: ensure ro / rw behavior is correct, CSIC-343. eg what if user specifies "rw" as a mountOption?
		mountOptions = append(mountOptions, "ro")
	}

	klog.V(4).Infof("nfs mount options are [%v]", mountOptions)

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
				klog.Error(e)
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
	targetPath := req.GetTargetPath()      // this is the path on the host node
	hostTargetPath := "/host" + targetPath // this is the path inside the csi container

	// Chown
	var uid_int, gid_int int
	tmp := req.GetVolumeContext()[common.SC_UID] // Returns an empty string if key not found
	if tmp == "" {
		uid_int = -1 // -1 means to not change the value
	} else {
		uid_int, err = strconv.Atoi(tmp)
		if err != nil || uid_int < -1 {
			e := fmt.Errorf("storage class specifies an invalid volume UID with value [%d]: %s", uid_int, err)
			klog.Error(e)
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
			klog.Error(e)
			return e
		}
	}

	err = os.Chown(hostTargetPath, uid_int, gid_int)
	if err != nil {
		e := fmt.Errorf("failed to chown path '%s': %v", hostTargetPath, err)
		klog.Error(e)
		return status.Errorf(codes.Internal, e.Error())
	}
	klog.V(4).Infof("chown mount %s uid=%d gid=%d", hostTargetPath, uid_int, gid_int)

	// Chmod
	unixPermissions := req.GetVolumeContext()[common.SC_UNIX_PERMISSIONS] // Returns an empty string if key not found
	if unixPermissions != "" {
		tempVal, err := strconv.ParseUint(unixPermissions, 8, 32)
		if err != nil {
			e := fmt.Errorf("failed to convert unix_permissions '%s' error: %s", unixPermissions, err.Error())
			klog.Error(e)
			return status.Errorf(codes.Internal, e.Error())
		}
		mode := uint(tempVal)
		err = os.Chmod(hostTargetPath, os.FileMode(mode))
		if err != nil {
			e := fmt.Errorf("failed to chmod path '%s' with perms %s: error: %v", hostTargetPath, unixPermissions, err)
			klog.Error(e)
			return status.Errorf(codes.Internal, e.Error())
		}
		klog.V(4).Infof("chmod mount %s perms=%s", hostTargetPath, unixPermissions)
	}

	// print out the target permissions
	logPermissions("", filepath.Dir(hostTargetPath))

	return nil
}

func logPermissions(note, hostTargetPath string) {
	// print out the target permissions
	cmd := exec.Command("ls", "-l", hostTargetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("error in doing ls command on %s error is  %s\n", hostTargetPath, err.Error())
	}
	klog.V(4).Infof("%s \nmount point permissions on %s ... %s", note, hostTargetPath, string(output))
}

func nfsSanityCheck(req *csi.CreateVolumeRequest, scParams map[string]string, optionalParams map[string]string, api api.Client) (capacity int64, err error) {
	params := req.GetParameters()

	err = validateStorageClassParameters(scParams, optionalParams, params, api)
	if err != nil {
		klog.Error(err)
		return capacity, status.Error(codes.InvalidArgument, err.Error())
	}

	capacity = int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		klog.Warningf("volume Minimum capacity should be greater 1 GB")
	}

	useChap := params[common.SC_USE_CHAP]
	if useChap != "" {
		klog.Warningf("useCHAP is not a valid storage class parameter for nfs or nfs-treeq")
	}

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("block access requested for %s PV %s", params[common.SC_STORAGE_PROTOCOL], req.GetName())
			klog.Error(e)
			return capacity, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	return capacity, err
}
