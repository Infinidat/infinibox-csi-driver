package common

import (
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/common"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type DiskInfo struct {
	RootDir     string `json:"rootdir"`
	MpathDevice string `json:"mpathdevice"`
	IsBlock     bool   `json:"isblock"`
	VolumeID    int    `json:"volumeid"`
}

func MountLogic(config DiskInfo, targetPath, devicePath, stagePath, fsType string, mountOptions []string, isBlock, readOnly bool) error {
	const function = "mountLogic"
	mounter := &mount.SafeFormatAndMount{
		Interface: mount.NewWithoutSystemd(""),
		Exec:      utilexec.New()}
	var mounted bool
	mntPoints, err := mounter.List()
	if err != nil {
		e := fmt.Errorf("%s - mounter.List error %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.Internal, e.Error())
	}

	var chrootPath = config.RootDir + targetPath
	zlog.Debug().Msgf("%s - mounter.List has %d, looking for %s", function, len(mntPoints), chrootPath)
	for i := range mntPoints {
		if mntPoints[i].Path == chrootPath {
			mounted = true
			break
		}
	}

	if mounted {
		zlog.Debug().Msgf("%s path: %s already mounted", function, chrootPath)
		return nil
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
		zlog.Debug().Msgf("%s - mounting raw block volume at given path %s", function, targetPath)
		zlog.Debug().Msgf("%s: mount point does not exist, creating mount point.", function)
		zlog.Debug().Msgf("%s: run: mkdir --parents --mode %s '%s' ", function, mode, filepath.Dir(targetPath))

		cmd := exec.Command("mkdir", "--parents", "--mode", mode, filepath.Dir(targetPath))
		err = cmd.Run()
		if err != nil {
			e := fmt.Errorf("%s: failed to mkdir '%s': error: %s", function, targetPath, err)
			zlog.Error().Msg(e.Error())
			return status.Error(codes.Internal, e.Error())
		}

		zlog.Debug().Msgf("%s: creating file: %s", function, chrootPath)
		_, err = os.Create(chrootPath)
		if err != nil {
			e := fmt.Errorf("%s - failed to create target path for raw bind mount: %q, err: %v", function, targetPath, err)
			zlog.Error().Msg(e.Error())
			return status.Error(codes.Internal, e.Error())
		}
		devicePath = strings.Replace(devicePath, config.RootDir, "", 1)

		options = append(options, "bind")

		if err := mounter.Mount(devicePath, targetPath, "", options); err != nil {
			e := fmt.Errorf("%s: failed to mount fc volume %s to %s, error %v", function, devicePath, targetPath, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		if err := CreateConfigFile(config, stagePath); err != nil {
			zlog.Error().Msgf("%s  - failed to save config with error: %v", function, err)
			return err
		}
		zlog.Debug().Msgf("%s volume mounted successfully", function)
	} else {
		// option B: local filesystem access
		zlog.Debug().Msgf("%s - mounting volume with filesystem at given path %s", function, targetPath)

		// Create mountPoint, if it does not exist.
		mountPoint := targetPath
		_, err := os.Stat(mountPoint)
		if err != nil {
			zlog.Error().Msgf("%s - stat - error %s", function, err.Error())
		}
		if os.IsNotExist(err) {
			zlog.Debug().Msgf("%s - mount point %s does not exist, creating mount point.", function, mountPoint)
			_, _, err := ExecCommand.Command("mkdir", fmt.Sprintf("--parents --mode %s '%s'", mode, mountPoint))
			if err != nil {
				zlog.Error().Msgf("%s - failed to mkdir '%s': %v", function, mountPoint, err)
				return err
			}
		} else {
			zlog.Debug().Msgf("%s - mkdir of mountPoint not required. '%s' already exists", function, mountPoint)
		}

		options = append(options, mountOptions...)

		// Persist here so that even if mount fails, the globalmount metadata json
		// file will contain an mpath to use during clean up.
		zlog.Debug().Msgf("%s - persist disk config to json file for later use, when detaching the disk", function)
		if err = CreateConfigFile(config, stagePath); err != nil {
			zlog.Error().Msgf("%s - failed to save config with error: %v", function, err)
			return err
		}

		if fsType == common.FS_TYPE_XFS {
			zlog.Debug().Msgf("%s - device %s is of type xfs, mounting using 'nouuid' option.", function, devicePath)
			options = append(options, "nouuid")
		}

		err = mounter.FormatAndMount(devicePath, targetPath, fsType, options)
		if err != nil {
			zlog.Error().Msgf("%s - mounter.FormatAndMount error. devicePath: %s, targetPath: %s, fsType: %s, error: %s", function, devicePath, targetPath, fsType, err)
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				zlog.Error().Msgf("%s - device %s is already mounted on %s", function, devicePath, mountPoint)
			} else {
				msg := fmt.Sprintf("%s - failed to mount volume %s [%s] to %s, err: %v", function, devicePath, fsType, targetPath, err)
				zlog.Error().Msg(msg)
				return status.Errorf(codes.Internal, "%s", msg)
			}
		}
	}
	return nil
}

func CreateConfigFile(conf DiskInfo, mnt string) error {
	const function = "createConfigFile"
	zlog.Debug().Msgf("%s - diskInfo: %v mnt: %s", function, conf, mnt)
	file := path.Join(conf.RootDir, mnt, strconv.Itoa(conf.VolumeID)+".json")

	fp, err := os.Create(file)
	if err != nil {
		e := fmt.Errorf("%s: failed creating persist file with error %v file %s", function, err, file)
		zlog.Error().Msg(e.Error())
		return e
	}
	defer func() {
		if err := fp.Close(); err != nil {
			zlog.Error().Msgf("%s error in Close() %s", function, err.Error())
		}
	}()

	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		e := fmt.Errorf("%s: failed creating persist file with error %v", function, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	zlog.Debug().Msgf("%s: created persist config file at path %s", function, file)
	return nil
}

func LoadDiskInfoFromFile(conf *DiskInfo, mnt string) error {
	const function = "loadDiskInfoFromFile"
	file := path.Join(conf.RootDir, mnt, strconv.Itoa(conf.VolumeID)+".json")
	zlog.Debug().Msgf("%s file [%s]", function, file)
	b, err := os.ReadFile(file)
	if err != nil {
		zlog.Error().Msgf("%s error in file read [%s]", function, err.Error())
	} else {
		zlog.Debug().Msgf("%s file content [%s]", function, string(b))
	}

	fp, err := os.Open(file)
	if err != nil {
		e := fmt.Errorf("%s - Open - file: %s error %s", function, file, err.Error())
		zlog.Error().Msg(e.Error())
		return e
	}
	defer func() {
		if err := fp.Close(); err != nil {
			zlog.Error().Msgf("error in Close() %s", err.Error())
		}
	}()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		e := fmt.Errorf("%s - Decode - error %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return e
	}
	return nil
}

// Unmount using targetPath and cleanup directories and files.
func UnmountAndCleanUp(targetPath string) (err error) {
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
		zlog.Error().Msg(err.Error())
		return err
	}
	if isMounted {
		// TODO - Should include volume ID
		err := fmt.Errorf("error: volume remains mounted at targetHostPath '%s'", targetHostPath)
		zlog.Error().Msg(err.Error())
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
		zlog.Error().Msg(err.Error())
		return err
	}

	if isADir {
		zlog.Debug().Msgf("targetHostPath '%s' is a directory, not a file", targetHostPath)
		if err := cleanupOldMountDirectory(targetHostPath); err != nil {
			zlog.Err(err)
			return err
		}
		zlog.Debug().Msgf("Successfully cleaned up directory based targetHostPath '%s'", targetHostPath)
		return nil
	}

	// not a directory
	zlog.Debug().Msgf("targetHostPath '%s' is a file, not a directory", targetHostPath)
	if removeMountErr := os.Remove(targetHostPath); removeMountErr != nil {
		err := fmt.Errorf("failed to Remove() path '%s': %v", targetHostPath, removeMountErr)
		zlog.Error().Msg(err.Error())
		return err
	}
	zlog.Debug().Msgf("Successfully cleaned up file based targetHostPath '%s'", targetHostPath)

	return nil
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
		zlog.Error().Msg(err.Error())
		return err
	}
	if !isMountEmpty {
		err := fmt.Errorf("error: mount/ directory at targetHostPath '%s' is not empty and may contain volume data", targetHostPath)
		zlog.Error().Msg(err.Error())
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
			zlog.Error().Msg(err.Error())
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

// IsDirEmpty Check if a directory is empty. Return an isEmpty boolean and an error.
func IsDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			zlog.Error().Msgf("error in Close() %s", err.Error())
		}
	}()

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
