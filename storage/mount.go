/*
Copyright 2025 Infinidat
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
	"fmt"
	"github.com/amitosw15/infinibox-csi-driver/common"
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

type diskInfo struct {
	RootDir     string `json:"rootdir"`
	MpathDevice string `json:"mpathdevice"`
	IsBlock     bool   `json:"isblock"`
	VolumeID    int    `json:"volumeid"`
}

func mountLogic(config diskInfo, targetPath, devicePath, stagePath, fsType string, mountOptions []string, isBlock, readOnly bool) error {
	mounter := &mount.SafeFormatAndMount{
		Interface: mount.NewWithoutSystemd(""),
		Exec:      utilexec.New()}
	var mounted bool
	mntPoints, err := mounter.List()
	if err != nil {
		e := fmt.Errorf("mountLogic - mounter.List error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.Internal, e.Error())
	}

	var chrootPath = config.RootDir + targetPath
	zlog.Debug().Msgf("mountLogic - mounter.List has %d, looking for %s", len(mntPoints), chrootPath)
	for i := range mntPoints {
		if mntPoints[i].Path == chrootPath {
			mounted = true
			break
		}
	}

	if mounted {
		zlog.Debug().Msgf("mountLogic path: %s already mounted", chrootPath)
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
		zlog.Debug().Msgf("mountLogic - mounting raw block volume at given path %s", targetPath)
		zlog.Debug().Msgf("mountLogic: mount point does not exist, creating mount point.")
		zlog.Debug().Msgf("mountLogic: run: mkdir --parents --mode %s '%s' ", mode, filepath.Dir(targetPath))

		cmd := exec.Command("mkdir", "--parents", "--mode", mode, filepath.Dir(targetPath))
		err = cmd.Run()
		if err != nil {
			e := fmt.Errorf("mountLogic: failed to mkdir '%s': error: %s", targetPath, err)
			zlog.Error().Msg(e.Error())
			return status.Error(codes.Internal, e.Error())
		}

		zlog.Debug().Msgf("mountLogic: creating file: %s", chrootPath)
		_, err = os.Create(chrootPath)
		if err != nil {
			e := fmt.Errorf("mountLogic - failed to create target path for raw bind mount: %q, err: %v", targetPath, err)
			zlog.Error().Msg(e.Error())
			return status.Error(codes.Internal, e.Error())
		}
		devicePath = strings.Replace(devicePath, config.RootDir, "", 1)

		options = append(options, "bind")

		if err := mounter.Mount(devicePath, targetPath, "", options); err != nil {
			e := fmt.Errorf("mountLogic: failed to mount fc volume %s to %s, error %v", devicePath, targetPath, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		if err := createConfigFile(config, stagePath); err != nil {
			zlog.Error().Msgf("mountLogic  - failed to save config with error: %v", err)
			return err
		}
		zlog.Debug().Msgf("mountLogic volume mounted successfully")
	} else {
		// option B: local filesystem access
		zlog.Debug().Msgf("mountLogic - mounting volume with filesystem at given path %s", targetPath)

		// Create mountPoint, if it does not exist.
		mountPoint := targetPath
		_, err := os.Stat(mountPoint)
		if err != nil {
			zlog.Error().Msgf("mountLogic - stat - error %s", err.Error())
		}
		if os.IsNotExist(err) {
			zlog.Debug().Msgf("mountLogic - mount point %s does not exist, creating mount point.", mountPoint)
			_, _, err := execCommand.Command("mkdir", fmt.Sprintf("--parents --mode %s '%s'", mode, mountPoint))
			if err != nil {
				zlog.Error().Msgf("mountLogic - failed to mkdir '%s': %v", mountPoint, err)
				return err
			}
		} else {
			zlog.Debug().Msgf("mountLogic - mkdir of mountPoint not required. '%s' already exists", mountPoint)
		}

		options = append(options, mountOptions...)

		// Persist here so that even if mount fails, the globalmount metadata json
		// file will contain an mpath to use during clean up.
		zlog.Debug().Msgf("mountLogic - persist disk config to json file for later use, when detaching the disk")
		if err = createConfigFile(config, stagePath); err != nil {
			zlog.Error().Msgf("mountLogic - failed to save config with error: %v", err)
			return err
		}

		if fsType == common.FS_TYPE_XFS {
			zlog.Debug().Msgf("mountLogic - device %s is of type xfs, mounting using 'nouuid' option.", devicePath)
			options = append(options, "nouuid")
		}

		err = mounter.FormatAndMount(devicePath, targetPath, fsType, options)
		if err != nil {
			zlog.Error().Msgf("mountLogic - mounter.FormatAndMount error. devicePath: %s, targetPath: %s, fsType: %s, error: %s", devicePath, targetPath, fsType, err)
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				zlog.Error().Msgf("mountLogic - device %s is already mounted on %s", devicePath, mountPoint)
			} else {
				msg := fmt.Sprintf("mountLogic - failed to mount volume %s [%s] to %s, err: %v", devicePath, fsType, targetPath, err)
				zlog.Error().Msg(msg)
				return status.Errorf(codes.Internal, "%s", msg)
			}
		}
	}
	return nil
}

func createConfigFile(conf diskInfo, mnt string) error {
	zlog.Debug().Msgf("createConfigFile - diskInfo: %v mnt: %s", conf, mnt)
	file := path.Join(conf.RootDir, mnt, strconv.Itoa(conf.VolumeID)+".json")

	fp, err := os.Create(file)
	if err != nil {
		e := fmt.Errorf("createConfigFile: failed creating persist file with error %v file %s", err, file)
		zlog.Error().Msg(e.Error())
		return e
	}
	defer func() {
		if err := fp.Close(); err != nil {
			zlog.Error().Msgf("error in Close() %s", err.Error())
		}
	}()

	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		e := fmt.Errorf("createConfigFile: failed creating persist file with error %v", err)
		zlog.Error().Msg(e.Error())
		return e
	}
	zlog.Debug().Msgf("createConfigFile: created persist config file at path %s", file)
	return nil
}

func loadDiskInfoFromFile(conf *diskInfo, mnt string) error {
	file := path.Join(conf.RootDir, mnt, strconv.Itoa(conf.VolumeID)+".json")
	zlog.Debug().Msgf("loadDiskInfoFromFile file [%s]", file)
	b, err := os.ReadFile(file)
	if err != nil {
		zlog.Error().Msgf("loadDiskInfoFromFile error in file read [%s]", err.Error())
	} else {
		zlog.Debug().Msgf("loadDiskInfoFromFile file content [%s]", string(b))
	}

	fp, err := os.Open(file)
	if err != nil {
		e := fmt.Errorf("loadDiskInfoFromFile - Open - file: %s error %s", file, err.Error())
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
		e := fmt.Errorf("loadDiskInfoFromFile - Decode - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return e
	}
	return nil
}
