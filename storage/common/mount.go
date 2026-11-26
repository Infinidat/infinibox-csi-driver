/*
Copyright 2026 Infinidat
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

package common

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/storage/luks"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type Mounter struct {
	ReadOnly     bool
	FsType       string
	MountOptions []string
	Mounter      *mount.SafeFormatAndMount
	Exec         utilexec.Interface
	DeviceUtil   util.DeviceUtil
	TargetPath   string
	StagePath    string
	IsBlock      bool
	Encryption   *luks.SecretRef // Secret with the LUKS passphrase, nil when the volume is not encrypted
}

type DiskInfo struct {
	RootDir     string `json:"rootdir"`
	MpathDevice string `json:"mpathdevice"`
	IsBlock     bool   `json:"isblock"`
	VolumeID    int    `json:"volumeid"`

	// set for encrypted volumes: the decrypted device, closed by NodeUnstageVolume, and the
	// Secret with its passphrase, read again by NodeExpandVolume
	LuksDevice                string `json:"luksdevice,omitempty"`
	EncryptionSecretName      string `json:"encryptionsecretname,omitempty"`
	EncryptionSecretNamespace string `json:"encryptionsecretnamespace,omitempty"`
}

// SetEncryption records that the volume is encrypted with the passphrase in the given
// Secret. It does nothing when ref is nil.
func (d *DiskInfo) SetEncryption(ref *luks.SecretRef) {
	if ref == nil {
		return
	}
	d.LuksDevice = luks.MapperName(d.VolumeID)
	d.EncryptionSecretName = ref.Name
	d.EncryptionSecretNamespace = ref.Namespace
}

func (d DiskInfo) encryptionSecret() *luks.SecretRef {
	if d.LuksDevice == "" {
		return nil
	}
	return &luks.SecretRef{Name: d.EncryptionSecretName, Namespace: d.EncryptionSecretNamespace}
}

func MountLogic(config DiskInfo, targetPath, devicePath, stagePath, fsType string, mountOptions []string, isBlock, readOnly bool) error {
	const function = "mountLogic"

	// quiet the klog output due to mount_linux.go logic producing erroneous log
	// messages on initialization
	beforeValue := flag.Lookup("v").Value.String()
	_ = flag.Set("v", "1")

	mounter := &mount.SafeFormatAndMount{
		Interface: mount.NewWithoutSystemd(""),
		Exec:      utilexec.New()}

	// return to previous klog verbosity
	_ = flag.Set("v", beforeValue)

	var mounted bool
	mntPoints, err := mounter.List()
	if err != nil {
		e := common.Errorf("%s - mounter.List error %s", function, err.Error())
		return status.Error(codes.Internal, e.Error())
	}

	var chrootPath = config.RootDir + targetPath
	slog.Debug(function, " mounter.List len", len(mntPoints), "looking for", chrootPath)
	for i := range mntPoints {
		if mntPoints[i].Path == chrootPath {
			mounted = true
			break
		}
	}

	if mounted {
		slog.Debug(function, "path already mounted", chrootPath)
		return nil
	}

	devicePath, err = openEncryptedDevice(config, devicePath, stagePath, readOnly)
	if err != nil {
		return err
	}

	mode := "0750"
	if readOnly {
		mode = "0550"
	}

	options := []string{}
	if readOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}

	if isBlock {
		// option A: raw block volume access
		slog.Debug(function, "mounting raw block volume at given path", targetPath)
		slog.Debug("mount point does not exist, creating mount point.")
		slog.Debug(function, "run: mkdir --parents --mode", mode, "targetPath", filepath.Dir(targetPath))

		cmd := exec.Command("mkdir", "--parents", "--mode", mode, filepath.Dir(targetPath))
		err = cmd.Run()
		if err != nil {
			e := common.Errorf("%s: failed to mkdir '%s': error: %s", function, targetPath, err)
			return status.Error(codes.Internal, e.Error())
		}

		if err := CreateConfigFile(config, stagePath); err != nil {
			slog.Error(function, "failed to save config with error:", err)
			return err
		}

		slog.Debug(function, "creating file:", chrootPath)
		_, err = os.Create(chrootPath)
		if err != nil {
			e := common.Errorf("%s - failed to create target path for raw bind mount: %q, err: %v", function, targetPath, err)
			return status.Error(codes.Internal, e.Error())
		}
		devicePath = strings.Replace(devicePath, config.RootDir, "", 1)

		options = append(options, "bind")

		slog.Debug(function, "sleeping 3s before mounting:", chrootPath)
		time.Sleep(time.Second * 3)
		LogPermissions("targetPath perms", "/host"+targetPath)
		LogPermissions("devicePath perms", "/host"+devicePath)

		// example:
		// mount -o rw,bind /host/dev/mapper/mpathai /host/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/csi-osv-d2930b0cd2/ddb3679c-9937-436d-93b7-f14844a05517

		_, _, err := ExecCommand.Command("mount", fmt.Sprintf("-o %s %s %s", strings.Join(options, ","), devicePath, targetPath))
		if err != nil {
			e := common.Errorf("%s: failed to mount fc volume %s to %s, error %v, options %v", function, devicePath, targetPath, err, options)
			slog.Error("error mounting block device", "error", e.Error())
			return e
		}

		slog.Debug("block volume mounted successfully")
		return nil
	}

	// option B: local filesystem access
	slog.Debug("mounting volume with filesystem at given", "path", targetPath)

	// Create mountPoint, if it does not exist.
	mountPoint := targetPath
	_, err = os.Stat(mountPoint)
	if err != nil {
		slog.Error("stat", "error", err.Error())
	}
	if os.IsNotExist(err) {
		slog.Debug(" mount point does not exist, creating mount point.", "mount point", mountPoint)
		_, _, err := ExecCommand.Command("mkdir", fmt.Sprintf("--parents --mode %s '%s'", mode, mountPoint))
		if err != nil {
			slog.Error("failed to mkdir", "mountPoint", mountPoint, "error", err)
			return err
		}
	} else {
		slog.Debug("mkdir of mountPoint not required. already exists", "mountPoint", mountPoint)
	}

	options = append(options, mountOptions...)

	// Persist here so that even if mount fails, the globalmount metadata json
	// file will contain an mpath to use during clean up.
	slog.Debug("persist disk config to json file for later use, when detaching the disk")
	if err = CreateConfigFile(config, stagePath); err != nil {
		slog.Error("failed to save config ", "error", err)
		return err
	}

	if fsType == common.FSTypeXFS {
		slog.Debug("device is of type xfs, mounting using 'nouuid' option.", "device", devicePath)
		options = append(options, "nouuid")
	}

	err = mounter.FormatAndMount(devicePath, targetPath, fsType, options)
	if err != nil {
		slog.Error("mounter.FormatAndMount error.", "devicePath", devicePath, "targetPath", targetPath, "fsType", fsType, "error", err)
		searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)

		if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
			slog.Error("device is already mounted", "device", devicePath, "mountPoint", mountPoint)
		} else {
			msg := fmt.Sprintf("%s - failed to mount volume %s [%s] to %s, err: %v", function, devicePath, fsType, targetPath, err)
			slog.Error(msg)
			return status.Errorf(codes.Internal, "%s", msg)
		}
	}
	return nil
}

// openEncryptedDevice returns the device to mount: devicePath itself, or for an encrypted
// volume its decrypted device, after opening it (and formatting it with LUKS if blank).
func openEncryptedDevice(config DiskInfo, devicePath, stagePath string, readOnly bool) (string, error) {
	ref := config.encryptionSecret()
	if ref == nil {
		return devicePath, nil
	}
	// persist first, so that NodeUnstageVolume finds the LUKS device to close even if a
	// later step fails
	if err := CreateConfigFile(config, stagePath); err != nil {
		return "", err
	}
	ctx := context.Background()
	passphrase, err := luks.GetPassphrase(ctx, *ref)
	if err != nil {
		return "", err
	}
	return luks.Open(ctx, luks.OpenOptions{
		Device:     strings.TrimPrefix(devicePath, config.RootDir),
		Name:       config.LuksDevice,
		Passphrase: passphrase,
		ReadOnly:   readOnly,
	})
}

// CloseEncryptedDevice closes the decrypted device of an encrypted volume, if it is open on
// this node. It must run before the device below it is detached. diskInfoLoaded tells
// whether config was read from the staging path: if not, the default device name of the
// volume is closed if it exists.
func CloseEncryptedDevice(config DiskInfo, diskInfoLoaded bool) error {
	name := config.LuksDevice
	if !diskInfoLoaded {
		name = luks.MapperName(config.VolumeID)
	}
	if name == "" {
		return nil
	}
	return luks.Close(context.Background(), name)
}

func CreateConfigFile(conf DiskInfo, mnt string) error {
	const function = "createConfigFile"
	slog.Debug("diskInfo", "config", conf, "mnt", mnt)
	filePath := path.Join(conf.RootDir, mnt, strconv.Itoa(conf.VolumeID)+".json")

	file, err := os.Create(filePath)
	if err != nil {
		return common.Errorf("%s: failed creating persist file with error %w file %s", function, err, filePath)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	encoder := json.NewEncoder(file)
	if err = encoder.Encode(conf); err != nil {
		return common.Errorf("%s: failed creating persist file with error %w", function, err)
	}
	slog.Debug("created persist config file", "path", filePath)
	return nil
}

func LoadDiskInfoFromFile(conf *DiskInfo, mnt string) error {
	const function = "loadDiskInfoFromFile"
	filePath := path.Join(conf.RootDir, mnt, strconv.Itoa(conf.VolumeID)+".json")
	slog.Debug(function, "file", filePath)
	b, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error("error in file read", "error", err.Error())
	} else {
		slog.Debug(function, "file content", string(b))
	}

	file, err := os.Open(filePath)
	if err != nil {
		return common.Errorf("%s - Open - file: %s error %w", function, filePath, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()
	decoder := json.NewDecoder(file)
	if err = decoder.Decode(conf); err != nil {
		return common.Errorf("%s - Decode - error %w", function, err)
	}
	return nil
}

// Unmount using targetPath and cleanup directories and files.
func UnmountAndCleanUp(targetPath string) (err error) {
	slog.Debug("unmounting and cleaning up", "path for targetPath", targetPath)

	// quiet the klog output due to mount_linux.go logic producing erroneous log
	// messages on initialization
	beforeValue := flag.Lookup("v").Value.String()
	_ = flag.Set("v", "1")

	mounter := mount.NewWithoutSystemd("")

	// return to previous klog verbosity
	_ = flag.Set("v", beforeValue)

	targetHostPath := path.Join("/host", targetPath)

	slog.Debug("unmounting ", "targetPath", targetPath)
	if err := mounter.Unmount(targetPath); err != nil {
		slog.Warn("failed to unmount", "targetPath", targetPath, "error", err)
	} else {
		slog.Debug("successfully unmounted", "targetPath", targetPath)
	}

	isMounted, isMountedErr := isMountedByListMethod(targetHostPath)
	if isMountedErr != nil {
		return common.Errorf("error: failed to check if targetHostPath '%s' is unmounted after unmounting %w", targetHostPath, isMountedErr)
	}
	if isMounted {
		return common.Errorf("error: volume remains mounted at targetHostPath '%s'", targetHostPath)
	}
	slog.Debug("verified is not mounted", "targetHostPath", targetHostPath)

	// Check if targetHostPath exists
	if _, err := os.Stat(targetHostPath); os.IsNotExist(err) {
		slog.Debug("targetHostPath does not exist and does not need to be cleaned up", "targetHostPath", targetHostPath)
		return nil
	}

	// Check if targetHostPath is a directory or a file
	isADir, isADirError := IsDirectory(targetHostPath)
	if isADirError != nil {
		return common.Errorf("failed to check if targetHostPath '%s' is a directory: %w", targetHostPath, isADirError)
	}

	if isADir {
		slog.Debug("targetHostPath is a directory, not a file", "targetHostPath", targetHostPath)
		if err := cleanupOldMountDirectory(targetHostPath); err != nil {
			return common.Errorf("%w", err)
		}
		slog.Debug("successfully cleaned up directory", "targetHostPath", targetHostPath)
		return nil
	}

	// not a directory
	slog.Debug("targetHostPath is a file, not a directory", "targetHostPath", targetHostPath)
	if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
		return common.Errorf("failed to Remove() path '%s': %w", targetHostPath, removeMountErr)
	}
	slog.Debug("successfully cleaned up file based", "targetHostPath", targetHostPath)

	return nil
}

func isMountedByListMethod(targetHostPath string) (bool, error) {
	// Use List() to search for mount matching targetHostPath
	// Each mount in the list has this example form:
	// {/dev/mapper/mpathn /host/var/lib/kubelet/pods/d2f8fcf0-f816-4008-b8fe-5d5f16c854d0/
	// volumes/kubernetes.io~csi/csi-f581f6711d/mount xfs [rw seclabel relatime
	// nouuid attr2 inode64 logbufs=8 logbsize=64k sunit=128 swidth=2048 noquota] 0 0}
	//
	// type MountPoint struct {
	//    Device string
	//    Path   string
	//    Type   string
	//    Opts   []string // Opts may contain sensitive mount options (like passwords)
	//    Freq   int
	//    Pass   int
	// }

	slog.Debug("checking mount path using mounter's List() and searching with", "targetHostPath", targetHostPath)

	// quiet the klog output due to mount_linux.go logic producing erroneous log
	// messages on initialization
	beforeValue := flag.Lookup("v").Value.String()
	_ = flag.Set("v", "1")

	mounter := mount.NewWithoutSystemd("")

	// return to previous klog verbosity
	_ = flag.Set("v", beforeValue)

	mountList, mountListErr := mounter.List()
	if mountListErr != nil {
		return true, common.Errorf("%w", mountListErr)
	}
	slog.Log(context.Background(), common.LevelTrace, "info", "mount path list", mountList)

	// Search list for targetHostPath
	isMountedByListMethod := false
	for _, mount := range mountList {
		if mount.Path == targetHostPath {
			isMountedByListMethod = true
			break
		}
	}
	slog.Debug("path is mounted", "targetHostPath", targetHostPath, "isMounted", isMountedByListMethod)
	return isMountedByListMethod, nil
}

func cleanupOldMountDirectory(targetHostPath string) error {
	ctx := context.Background()
	slog.Debug("cleaning up old mount directory", "targetHostPath", targetHostPath)
	isMountEmpty, isMountEmptyErr := IsDirEmpty(targetHostPath)
	// Verify mount/ directory is empty. Fail if mount/ is not empty as that may be volume data.
	if isMountEmptyErr != nil {
		return common.Errorf("failed IsDirEmpty() using targetHostPath '%s': %w", targetHostPath, isMountEmptyErr)
	}
	if !isMountEmpty {
		return common.Errorf("error: mount directory at targetHostPath '%s' is not empty and may contain volume data", targetHostPath)
	}
	slog.Log(ctx, common.LevelTrace, "verified that targetHostPath directory, aka mount path, is empty of files", "targetHostPath", targetHostPath)

	// Clean up mount/
	if _, statErr := os.Stat(targetHostPath); os.IsNotExist(statErr) {
		slog.Debug("mount point already removed", "targetHostPath", targetHostPath)
	} else {
		slog.Log(ctx, common.LevelTrace, "removing mount point", "targetHostPath", targetHostPath)
		if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
			return common.Errorf("after unmounting, failed to Remove() path '%s': %w", targetHostPath, removeMountErr)
		}
	}
	slog.Debug("removed mount point", "targetHostPath", targetHostPath)

	csiHostPath := strings.TrimSuffix(targetHostPath, "/mount")
	volData := "vol_data.json"
	volDataPath := filepath.Join(csiHostPath, volData)

	// Clean up csi-NNNNNNN/vol_data.json file
	if _, statErr := os.Stat(volDataPath); os.IsNotExist(statErr) {
		slog.Log(ctx, common.LevelTrace, "already removed", "volData", volData, "path", csiHostPath)
	} else {
		slog.Log(ctx, common.LevelTrace, "removing", "volData", volData, "path", volDataPath)
		if err := os.Remove(volDataPath); err != nil {
			slog.Warn("after unmounting, failed to remove", "volData", volData, "volDataPath", volDataPath, "error", err)
		}
		slog.Debug("successfully removed ", "volData", volData, "path", volDataPath)
	}

	// Clean up csi-NNNNNNN directory
	if _, statErr := os.Stat(csiHostPath); os.IsNotExist(statErr) {
		slog.Debug("csi volume directory already removed", "csihostPath", csiHostPath)
	} else {
		slog.Debug("removing CSI volume directory", "csiHostPath", csiHostPath)
		if err := os.Remove(csiHostPath); err != nil {
			slog.Error("after unmounting, failed to remove CSI volume directory", "csiHostPath", csiHostPath, "error", err)
		}
		slog.Debug("successfully removed CSI volume", "csiHostPath", csiHostPath)
	}
	return nil
}

// IsDirEmpty Check if a directory is empty. Return an isEmpty boolean and an error.
func IsDirEmpty(name string) (bool, error) {
	file, err := os.Open(name)
	if err != nil {
		return false, common.Errorf("%w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	_, err = file.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

// IsDirectory Determine if a file represented  by `path` is a directory or not.
func IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, common.Errorf("%w", err)
	}

	return fileInfo.IsDir(), err
}

func GetDiskMounter(req *csi.NodePublishVolumeRequest) (*Mounter, error) {
	reqVolCapability := req.GetVolumeCapability()

	// check accessMode - where we will eventually police R/W etc (CSIC-343)
	accessMode := reqVolCapability.GetAccessMode().GetMode() // GetAccessMode() guaranteed not nil from controller.go

	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	var fstype string
	mountOptions := []string{}
	blockVolCapability := reqVolCapability.GetBlock()

	readOnly := false
	isBlock := false

	if req.Readonly || accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		readOnly = true
		slog.Debug("MULTI_NODE_READER_ONLY AccessMode requested")
	}

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their FC device
		isBlock = false

		fstype = mountVolCapability.GetFsType()

		// mountOptions - could be nil
		mountOptions = mountVolCapability.GetMountFlags()

	} else if mountVolCapability == nil && blockVolCapability != nil {
		// option B. user wants block access to their FC device
		isBlock = true

		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			slog.Warn("accessmode MULTI_NODE_MULTI_WRITER requested for raw block volume, could be dangerous")
		}
	} else {
		return nil, common.Errorf("bad VolumeCapability parameters: both block and mount modes, for volume: %s", req.GetVolumeId())
	}

	var encryption *luks.SecretRef
	secretRef, encrypted, err := luks.SecretRefFromVolumeContext(req.GetVolumeContext())
	if err != nil {
		return nil, common.Errorf("volume %s: %w", req.GetVolumeId(), err)
	}
	if encrypted {
		encryption = &secretRef
	}

	// quiet the klog output due to mount_linux.go logic producing erroneous log
	// messages on initialization
	beforeValue := flag.Lookup("v").Value.String()
	_ = flag.Set("v", "1")

	m := &Mounter{
		IsBlock:      isBlock,
		ReadOnly:     readOnly,
		FsType:       fstype,
		MountOptions: mountOptions,
		Mounter:      &mount.SafeFormatAndMount{Interface: mount.NewWithoutSystemd(""), Exec: utilexec.New()},
		Exec:         utilexec.New(),
		DeviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
		TargetPath:   req.GetTargetPath(),
		StagePath:    req.GetStagingTargetPath(),
		Encryption:   encryption,
	}

	// return to previous klog verbosity
	_ = flag.Set("v", beforeValue)

	return m, nil
}
