package common

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"

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
	NFSMountOptionHard     = "hard"
	NFSMountOptionsSoft    = "soft"
	K8SMountPerms          = "020000775" // setgid bit
	StandardMountOptions   = "vers=3,tcp,rsize=262144,wsize=262144"
	NFSVersionRegex        = `(nfs){0,1}vers=([0-9]*)`
	NFSMountOptionReadonly = "ro"
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
		slog.Error("failed updateNfsMountOptions()", "error", err)
		return mountOptions, err
	}

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		mountOptions = append(mountOptions, NFSMountOptionReadonly)
	}

	slog.Debug("nfs mount options", "mountOptions", mountOptions)

	return mountOptions, nil
}

// SetVolumePermissions
func (sh StorageService) SetVolumePermissions(req *csi.NodePublishVolumeRequest) (err error) {
	// fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()
	// fsGroupIsSet := (fsGroup != "")
	// slog.Debug().Msgf("StorageHelper fsGroup: %s", fsGroup)

	UID := -1
	GID := -1

	tmp := req.GetVolumeContext()[common.StorageClassUID] // Returns an empty string if key not found
	if tmp != "" {
		UID, err = strconv.Atoi(tmp)
		if err != nil || UID < -1 {
			return common.Errorf("storage class specifies an invalid volume UID with value [%d]: %s", UID, err)
		}
	}

	tmp = req.GetVolumeContext()[common.StorageClassGID]
	if tmp != "" {
		GID, err = strconv.Atoi(tmp)
		if err != nil || GID < -1 {
			return common.Errorf("storage class specifies an invalid volume GID with value [%d]: %s", GID, err)
		}
	}

	targetPath := req.GetTargetPath()      // this is the path on the host node
	hostTargetPath := "/host" + targetPath // this is the path inside the csi container

	// chown the mount path with either a user supplied value or the fsGroup value
	if UID != -1 || GID != -1 {
		slog.Debug("user specified uid or gid in StorageClass parameters", "command", fmt.Sprintf("chown mount %s uid=%d gid=%d", hostTargetPath, UID, GID))
		err = os.Chown(hostTargetPath, UID, GID)
		if err != nil {
			return status.Error(codes.Internal, common.Errorf("failed to chown path '%s': %v", hostTargetPath, err).Error())
		}
	}

	unixPermissions := req.GetVolumeContext()[common.StorageClassUNIXPermissions]

	if unixPermissions != "" {
		slog.Debug("user specified unix_permissions in StorageClass parameters, chmod mount ", "hostTargetPath", hostTargetPath, "perms", unixPermissions)
		tempVal, err := strconv.ParseUint(unixPermissions, 8, 32)
		if err != nil {
			return status.Error(codes.Internal, common.Errorf("failed to convert unix_permissions '%s' error: %s", unixPermissions, err.Error()).Error())
		}
		mode := uint(tempVal)
		err = os.Chmod(hostTargetPath, os.FileMode(mode))
		if err != nil {
			return status.Error(codes.Internal, common.Errorf("failed to chmod path '%s' with perms %s: error: %v", hostTargetPath, unixPermissions, err).Error())
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
			slog.Error("performing recursive ownership change because reading permissions of root volume failed", "path", path, "error", err)
			return nil
		}
		stat, ok := fsInfo.Sys().(*syscall.Stat_t)
		if !ok || stat == nil {
			slog.Error("performing recursive ownership change because reading permissions of root volume failed", "path", path)
			return nil
		}
		slog.Debug("info", "Path", path, "volume gid", stat.Gid, "fsGroup", gid)
		// nothing to change if they match
		if int(stat.Gid) == gid {
			return nil
		}
		slog.Debug("expected group ownership of volume", "path", path, "did not match with", stat.Gid)
	}

	err := filepath.WalkDir(path,
		func(path string, dir fs.DirEntry, err error) error {
			if err == nil {
				slog.Log(context.Background(), common.LevelTrace, "Chown", "path", path, "uid", uid, "gid", gid)

				// handle the case on .snapshot hidden directories because they are readonly created by the ibox
				if snapdirVisible && dir.Name() == ".snapshot" {
					slog.Warn("Chown: skipping chown on", "dir", dir.Name(), "reason", "because snapdir_visible is true")
					return filepath.SkipDir
				}

				// handle the broken symlink case, skip chown on broken symlinks
				if dir.Type()&os.ModeSymlink != 0 {
					slog.Warn("Chown: we have a symlink!", "path", path)
					_, e := os.ReadFile(path)
					if e != nil {
						slog.Warn("Chown: error reading link, assuming its a broken link, skipping chown on it", "error", e.Error())
						return nil
					}
					slog.Warn("Chown: link is good ", "path", path)
				}

				err = os.Chown(path, uid, gid)
			}
			return err
		})

	slog.Debug("ChownR", "elapsed time", time.Since(start))
	return err
}

func LogPermissions(note, hostTargetPath string) {
	// print out the target permissions
	cmd := exec.Command("ls", "-l", hostTargetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("error in doing ls command", "path", hostTargetPath, "error", err.Error())
	}
	slog.Debug("info", "note", note, "hostTargetPath", hostTargetPath, "perms", string(output))
}

func UpdateNfsMountOptions(mountOptions []string, req *csi.NodePublishVolumeRequest) ([]string, error) {
	// If vers set to anything but 3 or 4 or 4.1, fail.
	re := regexp.MustCompile(NFSVersionRegex)
	for _, opt := range mountOptions {
		matches := re.FindStringSubmatch(opt)
		if len(matches) > 0 {
			version := matches[2]
			if version != "3" && version != "4" && version != "4.1" {
				return nil, common.Errorf("nfs version mount option '%s' encountered, but only NFS versions 3 and 4 are supported", opt)
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
		case NFSMountOptionHard:
			hardInMountOptions = true
		case NFSMountOptionsSoft:
			softInMountOptions = true
		}
	}
	if !hardInMountOptions && !softInMountOptions {
		mountOptions = append(mountOptions, NFSMountOptionHard)
	}

	if req.GetReadonly() {
		mountOptions = append(mountOptions, NFSMountOptionReadonly)
	}

	// remove duplicates from this list
	mountOptions = slices.Compact(mountOptions)

	return mountOptions, nil
}
