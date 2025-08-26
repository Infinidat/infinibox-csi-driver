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
	"infinibox-csi-driver/common"
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
		if err := createConfigFile(config, stagePath); err != nil {
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
			_, _, err := execCommand.Command("mkdir", fmt.Sprintf("--parents --mode %s '%s'", mode, mountPoint))
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
		if err = createConfigFile(config, stagePath); err != nil {
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

func createConfigFile(conf diskInfo, mnt string) error {
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

func loadDiskInfoFromFile(conf *diskInfo, mnt string) error {
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
