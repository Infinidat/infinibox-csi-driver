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
package iscsi

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/containerd/containerd/snapshots/devmapper/dmsetup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

const (
	devMapperDir                string = dmsetup.DevMapperDir // ie /dev/mapper/
	CHAP_INBOUND_USERNAME              = "security_chap_inbound_username"
	CHAP_INBOUND_SECRET                = "security_chap_inbound_secret"
	CHAP_OUTBOUND_USERNAME             = "security_chap_outbound_username"
	CHAP_OUTBOUND_SECRET               = "security_chap_outbound_secret"
	SECURITY_METHOD                    = "security_method"
	SECURITY_METHOD_NONE               = "NONE"
	SECURITY_METHOD_CHAP               = "CHAP"
	SECURITY_METHOD_MUTUAL_CHAP        = "MUTUAL_CHAP"
)

type iscsiDiskUnmounter struct {
	iscsiDiskInfo *iscsiDisk
	mounter       mount.Interface
	exec          utilexec.Interface // mount.Exec
}

type iscsiDiskMounter struct {
	*iscsiDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         utilexec.Interface
	deviceUtil   util.DeviceUtil
	targetPath   string
	stagePath    string
}

type iscsiTarget struct {
	Portals []string
	Iqn     string
}

type iscsiDisk struct {
	Lun           string            `json:"lun"`
	Iface         string            `json:"iface"`
	CHAPDiscovery bool              `json:"chapdiscovery"`
	CHAPSession   bool              `json:"chapsession"`
	Secret        map[string]string `json:"secret"`
	InitiatorName string            `json:"initiatorname"`
	VolName       string            `json:"volumename"`
	VolumeID      int               `json:"volumeid"`
	IsBlock       bool              `json:"isblock"`
	MpathDevice   string            `json:"mpathdevice"`
	Targets       []iscsiTarget     `json:"targets"`
}

type SessionDetails struct {
	protocol  string
	ipAddress string
	hostID    string
	iqn       string
}

const (
	USE_CHAP            = "chap"
	USE_CHAP_MUTUAL     = "mutual_chap"
	ISCSI_TCP_TRANSPORT = "tcp"
	CHAP_USERNAME       = "node.session.auth.username"
	CHAP_PASSWORD       = "node.session.auth.password"
	CHAP_USERNAME_IN    = "node.session.auth.username_in"
	CHAP_PASSWORD_IN    = "node.session.auth.password_in"
)

var (
	CHAPSessionCredentials = []string{
		CHAP_USERNAME,
		CHAP_PASSWORD,
		CHAP_USERNAME_IN,
		CHAP_PASSWORD_IN,
	}
	ifaceTransportNameRe = regexp.MustCompile(`iface.transport_name = (.*)\n`)
)

// Global resource contains a sync.Mutex. Used to serialize iSCSI resource accesses.
var execCommand helper.Exec

// StatFunc stat a path, if not exists, retry maxRetries times
// when iscsi transports other than default are used
type StatFunc func(string) (os.FileInfo, error)

// GlobFunc  use glob instead as pci id of device is unknown
type GlobFunc func(string) ([]string, error)

func (iscsi *ISCSIstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	const functionName = "NodeStageVolume"
	zlog.Debug().Msgf("%s (iscsi) called with publish context: %s %s", functionName, req.GetPublishContext(),
		storagecommon.GetHostInfo(req.GetSecrets(), iscsi.CS.IboxAPI))

	hostID, ports, err := storagecommon.ValidatePublishContext(req.GetPublishContext())
	if err != nil {
		e := fmt.Errorf("%s  (iscsi)- validatePublishContext - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostSecurity := req.GetPublishContext()["securityMethod"]
	useChap := req.GetVolumeContext()[common.StorageClassUseCHAP]
	zlog.Debug().Msgf("%s (iscsi) - Publishing volume to host with hostID %d", functionName, hostID)

	initiatorName := getInitiatorName()
	if initiatorName == "" {
		e := fmt.Errorf("%s (iscsi) - iscsi initiator name not found", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if !strings.Contains(ports, initiatorName) {
		zlog.Debug().Msgf("%s (iscsi) - host port is not created, creating one", functionName)
		err = iscsi.CS.AddPortForHost(hostID, "ISCSI", initiatorName)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - AddPortForHost - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	zlog.Debug().Msgf("%s (iscsi) - setup chap auth as '%s'", functionName, useChap)
	if strings.ToLower(hostSecurity) != useChap || !strings.Contains(ports, initiatorName) {
		secrets := req.GetSecrets()
		chapCreds := make(map[string]string)
		if useChap != "none" {
			if useChap == USE_CHAP || useChap == USE_CHAP_MUTUAL {
				if secrets[CHAP_USERNAME] != "" && secrets[CHAP_PASSWORD] != "" {
					chapCreds[CHAP_INBOUND_USERNAME] = secrets[CHAP_USERNAME]
					chapCreds[CHAP_INBOUND_SECRET] = secrets[CHAP_PASSWORD]
					chapCreds[SECURITY_METHOD] = SECURITY_METHOD_CHAP
				} else {
					e := fmt.Errorf("%s (iscsi) - iscsi mutual chap credentials not provided", functionName)
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
			if useChap == USE_CHAP_MUTUAL {
				if secrets[CHAP_USERNAME_IN] != "" && secrets[CHAP_PASSWORD_IN] != "" && chapCreds[SECURITY_METHOD] == SECURITY_METHOD_CHAP {
					chapCreds[CHAP_OUTBOUND_USERNAME] = secrets[CHAP_USERNAME_IN]
					chapCreds[CHAP_OUTBOUND_SECRET] = secrets[CHAP_PASSWORD_IN]
					chapCreds[SECURITY_METHOD] = SECURITY_METHOD_MUTUAL_CHAP
				} else {
					e := fmt.Errorf("%s (iscsi) - iscsi mutual chap credentials not provided", functionName)
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
			if len(chapCreds) > 1 {
				zlog.Debug().Msgf("%s (iscsi) - create chap authentication for host %d", functionName, hostID)
				err := addChapSecurityForHost(iscsi.CS, hostID, chapCreds)
				if err != nil {
					e := fmt.Errorf("%s (iscsi) - AddChapSecurityForHost - error: %s", functionName, err.Error())
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
		} else if hostSecurity != SECURITY_METHOD_NONE {
			zlog.Debug().Msgf("%s (iscsi) - remove chap authentication for host %d", functionName, hostID)
			chapCreds[SECURITY_METHOD] = SECURITY_METHOD_NONE
			err := addChapSecurityForHost(iscsi.CS, hostID, chapCreds)
			if err != nil {
				e := fmt.Errorf("%s (iscsi) - AddChapSecurityForHost - error: %s", functionName, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	const functionName = "NodePublishVolume"
	zlog.Debug().Msgf("%s (iscsi) - volume ID %s, network_space %s mode %s readOnly %t %s", functionName, req.GetVolumeId(), req.GetVolumeContext()[common.StorageClassNetworkSpace], req.GetVolumeCapability().GetAccessMode().Mode, req.Readonly,
		storagecommon.GetHostInfo(req.GetSecrets(), iscsi.CS.IboxAPI))

	targets, err := iscsi.getISCSITargets(req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - getISCSITargets - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (iscsi) iscsi %d targets  %v", functionName, len(targets), targets)

	iscsiDisk, err := iscsi.getISCSIDisk(req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - getISCSIDisk - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	iscsiDisk.Targets = targets
	zlog.Debug().Msgf("%s (iscsi) - iscsiDisk: vol name %s lun %s", functionName, iscsiDisk.VolName, iscsiDisk.Lun)

	diskMounter, err := iscsi.getISCSIDiskMounter(iscsiDisk, req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - getISCSIDiskMounter - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - AttachDisk - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (iscsi) - iscsi attachDisk succeeded", functionName)

	if diskMounter.readOnly {
		zlog.Debug().Msgf("%s (iscsi) - skipping chown-chmod since this is readOnly volume", functionName)
	} else {
		// Chown
		err = iscsi.StorageHelper.SetVolumePermissions(req)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - SetVolumePermissions - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume (iscsi) returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	const functionName = "NodeUnpublishVolume"
	zlog.Debug().Msgf("%s (iscsi) volume ID %s and targetPath '%s'", functionName, req.GetVolumeId(), req.GetTargetPath())

	err := storagecommon.UnmountAndCleanUp(req.GetTargetPath())
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - unmountAndCleanup - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {
	const functionName = "NodeUnstageVolume"
	zlog.Debug().Msgf("%s (iscsi) volume ID %s", functionName, req.GetVolumeId())

	diskUnmounter := iscsi.getISCSIDiskUnmounter()
	stagePath := req.GetStagingTargetPath()
	var mpathDevice string

	zlog.Debug().Msgf("%s (iscsi) - staging target path: %s", functionName, stagePath)

	// Load iscsi disk config from json file
	dskInfo := storagecommon.DiskInfo{
		VolumeID: diskUnmounter.iscsiDiskInfo.VolumeID,
		RootDir:  common.NodeRootDir,
	}
	if err := storagecommon.LoadDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		zlog.Debug().Msgf("%s (iscsi) - successfully loaded disk information from %s, mpath=[%s]", functionName, stagePath, mpathDevice)
	} else {
		// confFile := path.Join("/host", stagePath, diskUnmounter.iscsiDisk.VolName+".json")
		confFile := path.Join("/host", stagePath, strconv.Itoa(diskUnmounter.iscsiDiskInfo.VolumeID)+".json")
		zlog.Debug().Msgf("%s (iscsi) - check if config file exists", functionName)
		pathExist, pathErr := iscsi.CS.PathExists(confFile)
		if pathErr != nil {
			zlog.Error().Msgf("%s (iscsi) - pathExists - error: %s", functionName, pathErr.Error())
		}
		if pathErr == nil {
			if !pathExist {
				zlog.Debug().Msgf("%s (iscsi) - config file doesnt exist, calling RemoveAll with stagePath %s", functionName, stagePath)

				_ = storagecommon.DebugWalkDir(stagePath)

				// TODO - Review code
				if err := os.RemoveAll(stagePath); err != nil {
					zlog.Error().Msg(err.Error())
					zlog.Warn().Msgf("%s (iscsi) - failed to RemoveAll stage path '%s': %v", functionName, stagePath, err)
				}
				zlog.Debug().Msgf("%s (iscsi) - removed stage path '%s'", functionName, stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		zlog.Warn().Msgf("%s (iscsi) - failed to get iscsi config from stage path '%s': %v", functionName, stagePath, err)
	}

	// remove multipath
	err = storagecommon.DetachMpathDevice(mpathDevice, common.ProtocolISCSI)
	if err != nil {
		zlog.Warn().Msgf("%s (iscsi) - cannot detach volume with ID %s: %+v", functionName, req.GetVolumeId(), err)
	}

	removePath := path.Join("/host", stagePath)
	zlog.Debug().Msgf("%s (iscsi) - calling RemoveAll with removePath '%s'", functionName, removePath)

	_ = storagecommon.DebugWalkDir(removePath)

	// Check if removePath is a directory or a file
	isADir, isADirError := storagecommon.IsDirectory(removePath)
	if isADirError != nil {
		e := fmt.Errorf("%s (iscsi) - IsDirectory - check if removePath '%s' is a directory: %v", functionName, removePath, isADirError)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// Remove directory contents
	if isADir {
		// removePath '/host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount'
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount/93642552.json
		// 93642552.json: {"Portals":["172.31.32.145:3260","172.31.32.146:3260","172.31.32.147:3260","172.31.32.148:3260","172.31.32.149:3260","172.31.32.150:3260"],"Iqn":"iqn.2009-11.com.infinidat:storage:infinibox-sn-1521","Iface":"172.31.32.145:3260","InitiatorName":"iqn.1994-05.com.redhat:462c9b4cda1","VolName":"93642189","MpathDevice":"/dev/dm-8"}

		zlog.Debug().Msgf("%s (iscsi) - removePath '%s' is a directory", functionName, removePath)
		jsonPath := fmt.Sprintf("%s/%d.json", removePath, iscsi.CS.VolProto.VolumeID)
		zlog.Debug().Msgf("%s (iscsi) - removing json file '%s'", functionName, jsonPath)
		if err := os.Remove(jsonPath); err != nil {
			e := fmt.Errorf("%s (iscsi) - Remove remove json file %s error: %v", functionName, jsonPath, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Debug().Msgf("%s (iscsi) - removePath '%s' is not a directory", functionName, removePath)
	}

	// Remove directory or file
	zlog.Debug().Msgf("%s (iscsi) - removing removePath '%s'", functionName, removePath)
	if err := os.Remove(removePath); err != nil {
		e := fmt.Errorf("%s (iscsi) - Remote - failed to remove path '%s': %v", functionName, removePath, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// logout all iscsid sessions if there are zero devices, this stops iscid from
	// maintaining tcp connections to the ibox when there are zero devices

	// start by waiting a small amount of time to avoid a race condition with multipathd as
	// it takes it a bit to actually remove any devices we are checking against
	time.Sleep(time.Second * 3)

	deviceCount, err := getMultipathDeviceCount()
	if err != nil {
		zlog.Error().Msgf("%s (iscsi) - getMultipathDeviceCount - error: %s", functionName, err.Error())
	} else {
		zlog.Debug().Msgf("%s (iscsi) - multipath device count %d", functionName, deviceCount)
		if deviceCount == 0 {
			zlog.Debug().Msgf("%s (iscsi) - zero multipath devices - performing iscsi all sessions logout", functionName)
			err = logoutAllSessions()
			if err != nil {
				zlog.Error().Msgf("%s (iscsi) - iscsi logoutall error: %s", functionName, err.Error())
			}
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (iscsi) should never be called, called in node.go instead")
}

func (iscsi *ISCSIstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (iscsi *ISCSIstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (iscsi *ISCSIstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	const functionName = "NodeExpandVolume"
	zlog.Info().Msgf("%s (iscsi) called request volume ID %s path %s %s", functionName, req.GetVolumeId(), req.GetVolumePath(),
		storagecommon.GetHostInfo(req.GetSecrets(), iscsi.CS.IboxAPI))
	response := csi.NodeExpandVolumeResponse{}

	// the block volume case
	block := req.GetVolumeCapability().GetBlock()
	zlog.Debug().Msgf("%s (iscsi) - block string is [%s]", functionName, block.String())

	if req.GetVolumeCapability().GetBlock() != nil {
		err := storagecommon.BlockExpandVolume(req.GetVolumePath())
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - blockExpandVolume block volume name %s - error: %s", functionName, req.GetVolumePath(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		return &response, nil
	}

	// 1 - run mount | grep <volume_path> to find the multipath device name (e.g. /dev/mapper/mpathwi)
	multipathDevice, err := storagecommon.FindMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - findMultipathDevice - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	zlog.Debug().Msgf("%s (iscsi) - command is [%s]", functionName, command)

	out, _, err := execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - Command %s - error: %s", functionName, command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	if out == "" {
		e := fmt.Errorf("%s (iscsi) - error getting multipath devices from output %s command output was empty", functionName, multipathDevice)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	output := strings.TrimSpace(out)
	zlog.Debug().Msgf("%s (iscsi) - output is [%s]\n", functionName, output)
	outputParts := strings.Split(output, "\n")
	zlog.Debug().Msgf("%s (iscsi) - lines %d\n", functionName, len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for index := range outputParts {
		if outputParts[index] != "" {
			line := strings.Split(outputParts[index], "_")
			if len(line) < 2 {
				zlog.Error().Msgf("%s (iscsi) - error getting multipath blockDevice from output %v", functionName, line)
				continue
			}
			blockDevice := line[1]
			zlog.Debug().Msgf("device is [%s]\n", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, _, err := execCommand.Command(command, "")
			if err != nil {
				e := fmt.Errorf("%s (iscsi) - Command %s - error writing rescan on multipath devices %s", functionName, command, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, e
			}
			zlog.Debug().Msgf("%s (iscsi) rescan output is [%s]\n", functionName, strings.TrimSpace(out))
		}
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		return nil, fmt.Errorf("%s (iscsi) - error getting mpathPart from %+v", functionName, mpathPart)
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	zlog.Debug().Msgf("command is [%s]", command)
	out, _, err = execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s  (iscsi)- error multipathd resize map multipath devices %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]", strings.TrimSpace(out))

	// 5 - run resize2fs or xfs_growfs on /dev/mapper/mpathwi
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = storagecommon.ExpandFileSystem(multipathDevice, fsType)
	if err != nil {
		e := fmt.Errorf("%s  (iscsi)- error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	return &response, nil
}

func (iscsi *ISCSIstorage) AttachDisk(diskMounter iscsiDiskMounter) (mountPath string, err error) {
	var devicePath string
	var iscsiTransport string
	const functionName = "AttachDisk"

	zlog.Debug().Msgf("%s (iscsi), fsType: %s readOnly: %v mountOpts: %v targetPath: %s stagePath: %s",
		functionName, diskMounter.fsType, diskMounter.readOnly, diskMounter.mountOptions, diskMounter.targetPath, diskMounter.stagePath)

	zlog.Debug().Msgf("check that provided interface '%s' is available", diskMounter.Iface)
	isToLogOutput := false
	commandOutput, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", diskMounter.Iface), isToLogOutput)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) cannot read interface: %s output: %s error: %v", functionName, diskMounter.Iface, commandOutput, err)
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("%s (iscsi) provided interface '%s': ", functionName, diskMounter.Iface) // , out)

	iscsiTransport = iscsi.extractTransportName(commandOutput)
	zlog.Debug().Msgf("%s (iscsi) iscsiTransport: %s", functionName, iscsiTransport)
	if iscsiTransport == "" {
		e := fmt.Errorf("%s (iscsi) could not find transport name in iface %s", functionName, diskMounter.Iface) // TODO - b.Iface here really should be newIface...or does it matter?
		zlog.Error().Msg(e.Error())
		return "", e
	}

	// If not found, create new iface and copy parameters from pre-configured (default) iface to the created iface
	// Use one interface per iSCSI network-space, i.e. usually one per IBox.
	// TODO what does a blank Initiator name mean?
	targets := diskMounter.Targets
	if diskMounter.InitiatorName == "" {
		for _, target := range targets {
			// Look for existing interface named newIface. Clone default iface, if not found.
			zlog.Debug().Msgf("%s (iscsi) initiatorName: %s", functionName, diskMounter.InitiatorName)
			newIface := target.Portals[0] // Do not append ':$volume_id'
			zlog.Debug().Msgf("%s (iscsi) required iface name: '%s'", functionName, newIface)
			isToLogOutput := false
			_, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", newIface), isToLogOutput)
			if err != nil {
				zlog.Debug().Msgf("%s (iscsi) creating new iface (clone) and copying parameters from pre-configured iface to it", functionName)
				err = iscsi.cloneIface(diskMounter, newIface)
				if err != nil {
					e := fmt.Errorf("%s (iscsi) failed to clone iface: %s error: %v", functionName, diskMounter.Iface, err)
					zlog.Error().Msg(e.Error())
					return "", e
				}
				zlog.Debug().Msgf("%s (iscsi) new iface created '%s'", functionName, newIface)
			} else {
				zlog.Debug().Msgf("%s (iscsi) required iface '%s' already exists", functionName, newIface)
			}
		}
	} else {
		zlog.Debug().Msgf("%s (iscsi) Using existing initiator name'%s'", functionName, diskMounter.InitiatorName)
	}

	for _, target := range targets {
		for portalIndex := range target.Portals {
			zlog.Debug().Msgf("%s (iscsi) Discover targets at portal '%s'", functionName, target.Portals[portalIndex])
			// Discover all targets associated with a portal.
			_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode discoverydb --type sendtargets --portal %s --discover --op new --op delete", target.Portals[portalIndex]))
			if err != nil {
				e := fmt.Errorf("%s (iscsi) failed to discover targets at portal '%s': %v ", functionName, commandOutput, err)
				zlog.Error().Msgf("%s", e.Error())
				return "", e
			}
		}
	}

	if !diskMounter.CHAPSession {
		zlog.Debug().Msgf("%s (iscsi) target iqn: %s - Not using CHAP", functionName, targets[0].Iqn)
	} else {
		// Loop over portals:
		// - Set CHAP usage and update discoverydb with CHAP secret
		for index := range targets {
			for portalIndex := range targets[index].Portals {
				zlog.Debug().Msgf("%s (iscsi) target iface: %s iqn: %s - use CHAP at portal: %s", functionName, diskMounter.Iface, targets[index].Iqn, targets[index].Portals[portalIndex])
				err = iscsi.updateISCSINode(diskMounter, targets[index].Iqn, targets[index].Portals[portalIndex])
				if err != nil {
					zlog.Error().Msgf("%s", err.Error())
					// failure to update node db is rare. But deleting record will likely impact those who already start using it.
					zlog.Error().Msgf("%s", fmt.Errorf("%s (iscsi) : Failed to update iscsi node for portal: %s error: %v", functionName, targets[index].Portals[portalIndex], err.Error()))
					continue
				}
			}
		}
	}

	sessionDetails := getSessionDetails()
	zlog.Debug().Msgf("%s (iscsi) - list sessions before any logins: %v", functionName, sessionDetails)

	for index := range targets {
		// Check for at least one session. If none, login.
		zlog.Debug().Msgf("%s (iscsi) - list sessions to target iqn: %s", functionName, targets[index].Iqn)

		iqnFound := false
		for j := range sessionDetails {
			if sessionDetails[j].iqn == targets[index].Iqn {
				iqnFound = true
			}
		}
		if !iqnFound {
			for portal := range targets[index].Portals {
				zlog.Debug().Msgf("%s (iscsi) - login to iscsi target iqn %s at all portals using interface %s", functionName, targets[index].Iqn, targets[index].Portals[portal])
				_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --targetname %s --portal %s --login", targets[index].Iqn, targets[index].Portals[portal]))
				if err != nil {
					zlog.Error().Msg(err.Error())
					if status.Code(err) != codes.AlreadyExists {
						e := fmt.Errorf("%s (iscsi) - iscsi login failed to target iqn: %s, portal %s err: %s", functionName, targets[index].Iqn, targets[index].Portals[portal], err.Error())
						zlog.Error().Msg(e.Error())
						return "", e
					} else {
						zlog.Debug().Msgf("%s (iscsi) - already logged in to target iqn: %s portal %s", functionName, targets[index].Iqn, targets[index].Portals[portal])
					}
				}
			}
		} else {
			if len(targets[index].Portals) > 0 {
				zlog.Debug().Msgf("%s (iscsi) - already logged into iscsi target iqn %s using interface %s", functionName, targets[index].Iqn, targets[index].Portals[0])
			} else {
				zlog.Debug().Msgf("%s (iscsi) - already logged into iscsi target iqn %s", functionName, targets[index].Iqn)
			}
		}
	}
	sessionDetails = getSessionDetails()
	zlog.Debug().Msgf("%s (iscsi) - list sessions after any logins: %v", functionName, sessionDetails)

	// Rescan for LUN b.lun
	// Find hosts. TODO - take heed of portals.
	hosts, err := getHostIDs()
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - finding hosts failed: %s", functionName, err)
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("%s (iscsi) - hosts [%+v] len %d", functionName, hosts, len(hosts))

	// For each host, scan using lun

	wwid, err := storagecommon.RescanDeviceMap(hosts, diskMounter.VolName, diskMounter.Lun)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - rescanDeviceMap failed for volume ID %s and lun %s: %s", functionName, diskMounter.VolName, diskMounter.Lun, err)
		zlog.Error().Msg(e.Error())
		return "", e
	}

	if wwid == "" {
		e := fmt.Errorf("%s (iscsi) - searchDisk rescan error wwid not found", functionName)
		zlog.Error().Msg(e.Error())
		return "", e
	}

	zlog.Debug().Msgf("%s (iscsi) - searchDisk sleeping 3 seconds to allow devmapper time to work wwid [%s]", functionName, wwid)

	tries := 10 // currently this means a max of 10 seconds which is ample

	var dmDevice string
	for sleepIteration := range tries {
		dmDevice = storagecommon.GetDMDevicePath(wwid)
		if dmDevice != "" {
			zlog.Debug().Msgf("%s (iscsi) - found wwid '%s' and dm '%s'", functionName, wwid, dmDevice)
			break
		}
		if dmDevice != "" && strings.Contains(dmDevice, "dm-") {
			zlog.Debug().Msgf("%s (iscsi) - found a valid dm device [%s] iteration %d", functionName, dmDevice, sleepIteration)
			break
		}
		time.Sleep(time.Second * 1)
	}
	zlog.Debug().Msgf("%s (iscsi) - found dm [%s]", functionName, dmDevice)

	if dmDevice == "" {
		return "", fmt.Errorf("%s (iscsi) - error, could not find a dm device for wwid %s", functionName, wwid)
	}
	trimmedDeviceName := strings.Replace(dmDevice, "/host", "", 1)
	var thisMpath string
	thisMpath, err = storagecommon.FindMpathFromDevice(trimmedDeviceName)
	if err != nil {
		zlog.Error().Msgf("%s (iscsi) - findMpathFromDevice for [%s] error is [%s]", functionName, trimmedDeviceName, err.Error())
	}
	// here trimmedDeviceName is /dev/dm-3 and thisMpath is mpathtf
	zlog.Debug().Msgf("%s (iscsi) for [%s] thisMpath is [%s]", functionName, trimmedDeviceName, thisMpath)

	// Make sure we use a valid devicepath to find mpio device.
	// mntPath = b.targetPath
	// Mount device

	var options []string

	config := storagecommon.DiskInfo{
		VolumeID:    diskMounter.VolumeID,
		MpathDevice: thisMpath,
		RootDir:     common.NodeRootDir,
	}

	devicePath = devMapperDir + thisMpath

	err = storagecommon.MountLogic(config, diskMounter.targetPath, devicePath, diskMounter.stagePath, diskMounter.fsType, options, diskMounter.IsBlock, diskMounter.readOnly)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) mountLogic() failed, error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}

	zlog.Debug().Msgf("%s (iscsi) mounted volume with device path %s successfully at '%s'", functionName, devicePath, mountPath)
	return devicePath, nil
}

func getInitiatorName() string {
	cmd := "cat /etc/iscsi/initiatorname.iscsi | grep InitiatorName="
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		zlog.Error().Msgf("getInitiatorName (iscsi) - failed to get initiator name. Is iSCSI initiator installed? Error: %v", err)
		return ""
	}
	initiatorName := string(out)
	initiatorName = strings.TrimSuffix(initiatorName, "\n")
	zlog.Debug().Msgf("getInitiatorName (iscsi) - host initiator name %s ", initiatorName)
	arr := strings.Split(initiatorName, "=")
	return arr[1]
}

func (iscsi *ISCSIstorage) getISCSIDisk(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	initiatorName := getInitiatorName()

	volName := strconv.Itoa(iscsi.CS.VolProto.VolumeID)
	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	zlog.Debug().Msgf("getISCSIDisk (iscsi) volume: %d context: %v publish context: %v", iscsi.CS.VolProto.VolumeID, volContext, publishContext)

	lun := publishContext["lun"]
	if lun == "" {
		return nil, fmt.Errorf("getISCSIDisk (iscsi) iscsi: LUN is missing")
	}

	useChap := volContext[common.StorageClassUseCHAP]
	var chapSession bool
	if useChap != "none" {
		chapSession = true
	}
	var chapDiscovery bool
	if volContext["discoveryCHAPAuth"] == "true" {
		chapDiscovery = true
	}
	secret := req.GetSecrets()
	var err error
	if chapSession {
		secret, err = iscsi.parseSessionSecret(useChap, secret)
		if err != nil {
			e := fmt.Errorf("getISCSIDisk (iscsi) %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	return &iscsiDisk{
		VolumeID:      iscsi.CS.VolProto.VolumeID,
		VolName:       volName,
		Lun:           lun,
		Iface:         "default",
		CHAPDiscovery: chapDiscovery,
		CHAPSession:   chapSession,
		Secret:        secret,
		InitiatorName: initiatorName,
	}, nil
}

func (iscsi *ISCSIstorage) getISCSIDiskMounter(iscsiDisk *iscsiDisk, req *csi.NodePublishVolumeRequest) (*iscsiDiskMounter, error) {
	diskMounter := &iscsiDiskMounter{
		targetPath:   req.GetTargetPath(),
		stagePath:    req.GetStagingTargetPath(),
		mountOptions: []string{},
		mounter:      &mount.SafeFormatAndMount{Interface: mount.NewWithoutSystemd(""), Exec: utilexec.New()},
		exec:         utilexec.New(),
		deviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
	}

	// handle volumeCapabilities, the standard place to define block/file etc
	reqVolCapability := req.GetVolumeCapability()

	// check accessMode - where we will eventually police R/W etc (CSIC-343)
	accessMode := reqVolCapability.GetAccessMode().GetMode() // GetAccessMode() guaranteed not nil from controller.go

	if req.Readonly || accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		diskMounter.readOnly = true
	}
	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	blockVolCapability := reqVolCapability.GetBlock()

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their iSCSI device
		iscsiDisk.IsBlock = false

		diskMounter.fsType = mountVolCapability.GetFsType()

		// mountOptions - could be nothing
		diskMounter.mountOptions = mountVolCapability.GetMountFlags()

		// TODO: other validations needed for file?
		// - something about read-only access?
		// - check that fstype is supported?
		// - check that mount options are valid for fstype provided
	} else if mountVolCapability == nil && blockVolCapability != nil {
		// option B. user wants block access to their iSCSI device
		iscsiDisk.IsBlock = true

		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			zlog.Warn().Msg("getISCSIDiskMounter (iscsi) MULTI_NODE_MULTI_WRITER AccessMode requested for raw block volume, could be dangerous")
		}
		// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?

		// don't need to look at FsType or MountFlags here, only relevant for mountVol
		// TODO: other validations needed for block?
		// - something about read-only access?
	} else {
		errMsg := "getISCSIDiskMounter (iscsi) Bad VolumeCapability parameters: both block and mount modes, for volume: " + req.GetVolumeId()
		zlog.Error().Msg(errMsg)
		return nil, status.Error(codes.InvalidArgument, errMsg)
	}

	diskMounter.iscsiDisk = iscsiDisk

	return diskMounter, nil
}

func (iscsi *ISCSIstorage) getISCSIDiskUnmounter() *iscsiDiskUnmounter {
	return &iscsiDiskUnmounter{
		iscsiDiskInfo: &iscsiDisk{
			VolName:  strconv.Itoa(iscsi.CS.VolProto.VolumeID),
			VolumeID: iscsi.CS.VolProto.VolumeID,
		},
		mounter: mount.NewWithoutSystemd(""),
		exec:    utilexec.New(),
	}
}

func (iscsi *ISCSIstorage) parseSessionSecret(useChap string, secretParams map[string]string) (map[string]string, error) {
	var valid bool
	const functionName = "parseSessionSecret"
	secret := make(map[string]string)

	if useChap == USE_CHAP || useChap == USE_CHAP_MUTUAL {
		if len(secretParams) == 0 {
			return secret, errors.New("parseSessionSecret (iscsi): required chap secrets not provided")
		}
		if secret[CHAP_USERNAME], valid = secretParams[CHAP_USERNAME]; !valid {
			return secret, fmt.Errorf("%s (iscsi): %s not found in secret", functionName, CHAP_USERNAME)
		}
		if secret[CHAP_PASSWORD], valid = secretParams[CHAP_PASSWORD]; !valid {
			return secret, fmt.Errorf("%s (iscsi): %s not found in secret", functionName, CHAP_PASSWORD)
		}
		if useChap == USE_CHAP_MUTUAL {
			if secret[CHAP_USERNAME_IN], valid = secretParams[CHAP_USERNAME_IN]; !valid {
				return secret, fmt.Errorf("%s (iscsi): %s not found in secret", functionName, CHAP_USERNAME_IN)
			}
			if secret[CHAP_PASSWORD_IN], valid = secretParams[CHAP_PASSWORD_IN]; !valid {
				return secret, fmt.Errorf("%s(iscsi): %s not found in secret", functionName, CHAP_PASSWORD_IN)
			}
		}
		secret["SecretsType"] = USE_CHAP
	}
	return secret, nil
}

func (iscsi *ISCSIstorage) updateISCSINode(diskMounter iscsiDiskMounter, iqn string, portal string) error {
	const functionName = "updateISCSINode"
	if !diskMounter.CHAPSession {
		return nil
	}

	zlog.Debug().Msgf("%s (iscsi) update node with CHAP", functionName)
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name node.session.auth.authmethod --value CHAP", portal, iqn))
	if err != nil {
		e := fmt.Errorf("%s (iscsi): failed to update node with CHAP, output: %v", functionName, out)
		zlog.Error().Msg(e.Error())
		return e
	}

	for _, credential := range CHAPSessionCredentials {
		v := diskMounter.Secret[credential]
		if len(v) > 0 {
			zlog.Debug().Msgf("%s (iscsi) update node session key/value", functionName)
			out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name %q --value %q", portal, iqn, credential, v))
			if err != nil {
				e := fmt.Errorf("%s (iscsi): failed to update node session key %q with value %q error: %v", functionName, credential, v, out)
				zlog.Error().Msg(e.Error())
				return e
			}
		}
	}
	return nil
}

func (iscsi *ISCSIstorage) extractTransportName(ifaceOutput string) (iscsiTransport string) {
	rexOutput := ifaceTransportNameRe.FindStringSubmatch(ifaceOutput)
	if rexOutput == nil {
		return ""
	}
	iscsiTransport = rexOutput[1]

	// While iface.transport_name is a required parameter, handle it being unspecified anyways
	if iscsiTransport == "<empty>" {
		iscsiTransport = ISCSI_TCP_TRANSPORT
	}
	return iscsiTransport
}

func (iscsi *ISCSIstorage) parseIscsiadmShow(output string) (map[string]string, error) {
	params := make(map[string]string)
	slice := strings.Split(output, "\n")
	for _, line := range slice {
		if !strings.HasPrefix(line, "iface.") || strings.Contains(line, "<empty>") {
			continue
		}
		iface := strings.Fields(line)
		if len(iface) != 3 || iface[1] != "=" {
			e := fmt.Errorf("parseIscsiadmShow (iscsi) invalid iface setting %v", iface)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		// iscsi_ifacename is immutable once the iface is created
		if iface[0] == "iface.iscsi_ifacename" {
			continue
		}
		params[iface[0]] = iface[2]
	}
	return params, nil
}

func (iscsi *ISCSIstorage) cloneIface(diskMounter iscsiDiskMounter, newIface string) error {
	var lastErr error
	const functionName = "cloneIface"
	zlog.Debug().Msgf("%s (iscsi) - find pre-configured iface records", functionName)
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", diskMounter.Iface))
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		lastErr = fmt.Errorf("%s (iscsi): failed to show iface records: %s (%v)", functionName, out, err)
		return lastErr
	}
	zlog.Debug().Msgf("%s (iscsi) - pre-configured iface records found: %s", functionName, out)

	// parse obtained records
	params, err := iscsi.parseIscsiadmShow(out)
	if err != nil {
		zlog.Error().Msgf("%s (iscsi) - parse - error: %s", functionName, err.Error())
		lastErr = fmt.Errorf("%s (iscsi): Failed to parse iface records: %s (%v)", functionName, out, err)
		return lastErr
	}
	// update initiatorname
	params["iface.initiatorname"] = diskMounter.InitiatorName

	zlog.Debug().Msgf("%s (iscsi) - create new interface", functionName)
	out, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op new", newIface))
	if err != nil {
		lastErr = fmt.Errorf("%s (iscsi): failed to create new iface: %s (%v)", functionName, out, err)
		return lastErr
	}

	// update new iface records
	for key, val := range params {
		zlog.Debug().Msgf("%s (iscsi) - update records of interface '%s'", functionName, newIface)
		_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op update --name %q --value %q", newIface, key, val))
		if err != nil {
			zlog.Error().Msgf("%s", err.Error())
			_, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op delete", newIface))
			if err != nil {
				lastErr = fmt.Errorf("%s (iscsi) - failed to delete iface '%s': %s ", functionName, newIface, err)
				return lastErr
			}

			lastErr = fmt.Errorf("%s (iscsi): failed to update iface records: %s (%v). iface(%s) will be used", functionName, out, err, diskMounter.Iface)
			break
		}
	}
	return lastErr
}

func (iscsi *ISCSIstorage) getISCSITargets(req *csi.NodePublishVolumeRequest) (targets []iscsiTarget, err error) {
	const functionName = "getISCSITargets"
	networkSpaces := strings.Split(req.GetVolumeContext()[common.StorageClassNetworkSpace], ",")
	if len(networkSpaces) == 0 {
		return targets, fmt.Errorf("%s (iscsi) no network spaces found", functionName)
	}
	zlog.Debug().Msgf("networkSpaces %v", networkSpaces)
	if iscsi.CS.API == nil {
		return targets, fmt.Errorf("%s (iscsi) no api found", functionName)
	}

	var portalsExist bool
	targets = make([]iscsiTarget, len(networkSpaces))

	for index, networkSpace := range networkSpaces {
		zlog.Debug().Msgf("%s (iscsi) - getting nspace by name: %v", functionName, networkSpace)
		thisNetworkSpace, err := iscsi.CS.IboxAPI.GetNetworkSpaceByName(networkSpace)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - error getting network space: %s error: %v", functionName, networkSpace, err)
			zlog.Error().Msg(e.Error())
			return targets, status.Error(codes.InvalidArgument, e.Error())
		}
		zlog.Debug().Msgf("%s (iscsi) - got nspace by name: %s", functionName, thisNetworkSpace.Name)

		targets[index] = iscsiTarget{
			Iqn:     thisNetworkSpace.Properties.ISCSIIqn,
			Portals: []string{},
		}
		for _, portal := range thisNetworkSpace.Portals {
			if !portal.Enabled {
				zlog.Error().Msgf("%s (iscsi) - network space %s ip address %s is disabled, not adding to list of available ip addresses", functionName, thisNetworkSpace.Name, portal.IPAddress)
				continue
			}

			err := iscsi.StorageHelper.ValidateIPAddress(portal.IPAddress, thisNetworkSpace.Properties.ISCSITCPPort)
			if err != nil {
				zlog.Error().Msgf("%s (iscsi) - error getting iscsi network space %s ip connection to %s %d error: %v", functionName, networkSpace, portal.IPAddress, thisNetworkSpace.Properties.ISCSITCPPort, err)
				continue
			}

			zlog.Debug().Msgf("%s (iscsi) - adding iscsi network space %s ip connection to %s %d list", functionName, networkSpace, portal.IPAddress, thisNetworkSpace.Properties.ISCSITCPPort)
			targets[index].Portals = append(targets[index].Portals, storagecommon.PortalMounter(portal.IPAddress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, fmt.Errorf("%s (iscsi) - there are zero network space ip addresses available", functionName)
	}
	return targets, nil
}

func getSessionDetails() (results []SessionDetails) {
	rawOutput, _, err := execCommand.Command("iscsiadm", "--mode session")
	if err != nil {
		zlog.Error().Msgf("getISCSITargets (iscsi) - session list failed, error: %v", err)
		return results
	}
	lines, err := storagecommon.StringToLines(rawOutput)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return results
	}
	results = make([]SessionDetails, 0)
	for index := range lines {
		if len(lines[index]) > 0 {
			parts := strings.Split(lines[index], " ")
			protocolParts := strings.Split(parts[0], ":")
			ipaddressParts := strings.Split(parts[2], ",")
			session := SessionDetails{
				protocol:  protocolParts[0],
				ipAddress: ipaddressParts[0],
				hostID:    ipaddressParts[1],
				iqn:       parts[3],
			}
			results = append(results, session)
		}
	}

	return results
}

func getMultipathDeviceCount() (deviceCount int, err error) {
	command := "multipath -ll -v 1"
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)

	var out []byte
	out, err = exec.Command("bash", "-c", pipefailCmd).CombinedOutput()
	if err != nil {
		e := fmt.Errorf("getMultipathDeviceCount (iscsi) - multipath command error: %s", err)
		return deviceCount, e
	}
	devices := strings.Fields(string(out))
	zlog.Debug().Msgf("getMultipathDeviceCount (iscsi) - multipath output %s", string(out))
	return len(devices), nil
}

func logoutAllSessions() (err error) {
	command := "iscsiadm --mode node --logoutall=all"
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)

	var out []byte
	out, err = exec.Command("bash", "-c", pipefailCmd).CombinedOutput()
	if err != nil {
		return err
	}
	zlog.Debug().Msgf("logoutAllSessions (iscsi) isciadm logoutall output %s", string(out))
	return nil
}

func getHostIDs() (hosts []string, err error) {
	rawOutput, _, err := execCommand.Command("iscsiadm", "-m host -P0")
	if err != nil {
		e := fmt.Errorf("getHostIDs (iscsi) - finding hosts failed: %s", err)
		zlog.Error().Msg(e.Error())
		return hosts, e
	}

	// this raw output should look like this
	/**
		root@csi-test:~# iscsiadm -m host -P0
	tcp: [34] 172.20.65.31,[<empty>],<empty> <empty>
	tcp: [35] 172.20.65.31,[<empty>],<empty> <empty>
	tcp: [36] 172.20.65.31,[<empty>],<empty> <empty>
	tcp: [37] 172.20.65.31,[<empty>],<empty> <empty>
	tcp: [38] 172.20.65.31,[<empty>],<empty> <empty>
	tcp: [39] 172.20.65.31,[<empty>],<empty> <empty>

	the returned parsed string array should be ["34","35","36","37,"38","39"]
	*/

	parts := strings.Split(rawOutput, "\n")
	for index := range parts {
		if strings.Contains(parts[index], "tcp") {
			trim := strings.TrimLeft(parts[index], " ")
			rawNumber := strings.Split(trim, " ")
			if len(rawNumber) < 2 {
				err = fmt.Errorf("getHostIDs (iscsi) - error, could not parse host number [%s]", trim)
				return hosts, err
			}
			replaced := strings.ReplaceAll(rawNumber[1], "[", "")
			hostID := strings.ReplaceAll(replaced, "]", "")
			hosts = append(hosts, hostID)
		}
	}

	return hosts, nil
}

func addChapSecurityForHost(cs storagecommon.Commonservice, hostID int, credentials map[string]string) error {
	_, err := cs.IboxAPI.AddHostSecurity(credentials, hostID)
	if err != nil {
		zlog.Error().Msgf("failed to add authentication for host %d with error %v", hostID, err)
		return err
	}
	return nil
}
