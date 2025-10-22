package common

import (
	"fmt"
	"infinibox-csi-driver/common"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DEFAULT_FS_GROUP_CHANGE_POLICY = "Always" // we currently only support "Always", not "OnRootMisMatch"
// these particular storage functions get mocked and used in unit tests
type StorageHelper interface {
	SetVolumePermissions(req *csi.NodePublishVolumeRequest) (err error)
	ValidateIPAddress(ipAddress string, port int) (err error)
	GetNFSMountOptions(req *csi.NodePublishVolumeRequest) ([]string, error)
}

// When you ask "why?": https://github.com/golang/go/issues/25539#issuecomment-394615058
const (
	NFS_MOUNT_OPTION_HARD     = "hard"
	NFS_MOUNT_OPTION_SOFT     = "soft"
	K8S_MOUNT_PERMS           = "020000775" // setgid bit
	StandardMountOptions      = "vers=3,tcp,rsize=262144,wsize=262144"
	NFS_VERSION_REGEX         = `(nfs){0,1}vers=([0-9]*)`
	NFS_MOUNT_OPTION_READONLY = "ro"
)

type StorageService struct{}

// shanked from K8s
// rwMask   = os.FileMode(0660)
// roMask   = os.FileMode(0440)
// execMask = os.FileMode(0110)

func (sh StorageService) GetNFSMountOptions(req *csi.NodePublishVolumeRequest) (mountOptions []string, err error) {
	// Get mount options from VolumeCapability - the standard way
	mountOptions = req.GetVolumeCapability().GetMount().GetMountFlags()
	if len(mountOptions) == 0 {
		for _, option := range strings.Split(StandardMountOptions, ",") {
			if option != "" {
				mountOptions = append(mountOptions, option)
			}
		}
	}

	mountOptions, err = UpdateNfsMountOptions(mountOptions, req)
	if err != nil {
		zlog.Error().Msgf("failed updateNfsMountOptions(): %s", err)
		return mountOptions, err
	}

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		mountOptions = append(mountOptions, NFS_MOUNT_OPTION_READONLY)
	}

	zlog.Debug().Msgf("nfs mount options are [%v]", mountOptions)

	return mountOptions, nil
}

// SetVolumePermissions
func (sh StorageService) SetVolumePermissions(req *csi.NodePublishVolumeRequest) (err error) {
	// fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()
	// fsGroupIsSet := (fsGroup != "")
	// zlog.Debug().Msgf("StorageHelper fsGroup: %s", fsGroup)

	UID := -1
	GID := -1

	tmp := req.GetVolumeContext()[common.StorageClassUID] // Returns an empty string if key not found
	if tmp != "" {
		UID, err = strconv.Atoi(tmp)
		if err != nil || UID < -1 {
			e := fmt.Errorf("storage class specifies an invalid volume UID with value [%d]: %s", UID, err)
			zlog.Err(e)
			return e
		}
	}

	tmp = req.GetVolumeContext()[common.StorageClassGID]
	if tmp != "" {
		GID, err = strconv.Atoi(tmp)
		if err != nil || GID < -1 {
			e := fmt.Errorf("storage class specifies an invalid volume GID with value [%d]: %s", GID, err)
			zlog.Err(e)
			return e
		}
	}

	targetPath := req.GetTargetPath()      // this is the path on the host node
	hostTargetPath := "/host" + targetPath // this is the path inside the csi container

	// chown the mount path with either a user supplied value or the fsGroup value
	if UID != -1 || GID != -1 {
		zlog.Debug().Msgf("user specified uid or gid in StorageClass parameters, chown mount %s uid=%d gid=%d", hostTargetPath, UID, GID)
		err = os.Chown(hostTargetPath, UID, GID)
		if err != nil {
			e := fmt.Errorf("failed to chown path '%s': %v", hostTargetPath, err)
			zlog.Err(e)
			return status.Error(codes.Internal, e.Error())
		}
	}

	unixPermissions := req.GetVolumeContext()[common.StorageClassUNIXPermissions]

	if unixPermissions != "" {
		zlog.Debug().Msgf("user specified unix_permissions in StorageClass parameters, chmod mount %s perms=%s", hostTargetPath, unixPermissions)
		tempVal, err := strconv.ParseUint(unixPermissions, 8, 32)
		if err != nil {
			e := fmt.Errorf("failed to convert unix_permissions '%s' error: %s", unixPermissions, err.Error())
			zlog.Err(e)
			return status.Error(codes.Internal, e.Error())
		}
		mode := uint(tempVal)
		err = os.Chmod(hostTargetPath, os.FileMode(mode))
		if err != nil {
			e := fmt.Errorf("failed to chmod path '%s' with perms %s: error: %v", hostTargetPath, unixPermissions, err)
			zlog.Err(e)
			return status.Error(codes.Internal, e.Error())
		}
	}

	// print out the target permissions
	LogPermissions("", filepath.Dir(hostTargetPath))

	return nil
}

// recursively chowns a root path - currently not used as we let kubelet do fsGroup recursive permissions changes
func ChownR(path string, uid int, gid int, fsGroupIsSet bool, fsGroupChangePolicy string, snapdirVisible bool) error {
	start := time.Now()

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
		func(path string, dir fs.DirEntry, err error) error {
			if err == nil {
				zlog.Trace().Msgf("Chown: %s with uid: %d and gid: %d", path, uid, gid)

				// handle the case on .snapshot hidden directories because they are readonly created by the ibox
				if snapdirVisible && dir.Name() == ".snapshot" {
					zlog.Warn().Msgf("Chown: skipping chown on %s because snapdir_visible is true", dir.Name())
					return filepath.SkipDir
				}

				// handle the broken symlink case, skip chown on broken symlinks
				if dir.Type()&os.ModeSymlink != 0 {
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

	zlog.Debug().Msgf("ChownR elapsed time %v", time.Since(start))
	return err
}

func LogPermissions(note, hostTargetPath string) {
	// print out the target permissions
	cmd := exec.Command("ls", "-l", hostTargetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		zlog.Error().Msgf("error in doing ls command on %s error is  %s\n", hostTargetPath, err.Error())
	}
	zlog.Debug().Msgf("%s \nmount point permissions on %s ... %s", note, hostTargetPath, string(output))
}

func UpdateNfsMountOptions(mountOptions []string, req *csi.NodePublishVolumeRequest) ([]string, error) {
	// If vers set to anything but 3 or 4 or 4.1, fail.
	re := regexp.MustCompile(NFS_VERSION_REGEX)
	for _, opt := range mountOptions {
		matches := re.FindStringSubmatch(opt)
		if len(matches) > 0 {
			version := matches[2]
			if version != "3" && version != "4" && version != "4.1" {
				e := fmt.Errorf("nfs version mount option '%s' encountered, but only NFS versions 3 and 4 are supported", opt)
				zlog.Err(e)
				return nil, e
			}
		}
	}

	// Force vers=3 to be in the mountOptions slice if a vers is not explicitly set in the StorageClass. IBoxes require NFS version 3 or 4.
	versInMountOptions := false
	for _, opt := range mountOptions {
		if opt == "vers=3" || opt == "nfsvers=3" || opt == "vers=4" || opt == "nfsvers=4" || opt == "vers=4.1" || opt == "nfsvers=4.1" {
			versInMountOptions = true
			break
		}
	}
	if !versInMountOptions {
		mountOptions = append(mountOptions, "vers=3")
	}

	// Add option hard if 'soft' not set explicitly.
	var hardInMountOptions bool
	var softInMountOptions bool
	for _, opt := range mountOptions {
		switch opt {
		case NFS_MOUNT_OPTION_HARD:
			hardInMountOptions = true
		case NFS_MOUNT_OPTION_SOFT:
			softInMountOptions = true
		}
	}
	if !hardInMountOptions && !softInMountOptions {
		mountOptions = append(mountOptions, NFS_MOUNT_OPTION_HARD)
	}

	// Support readonly mount option.
	if req.GetReadonly() {
		// TODO: ensure ro / rw behavior is correct, CSIC-343. eg what if user specifies "rw" as a mountOption?
		mountOptions = append(mountOptions, NFS_MOUNT_OPTION_READONLY)
	}

	// remove duplicates from this list
	mountOptions = slices.Compact(mountOptions)

	return mountOptions, nil
}
