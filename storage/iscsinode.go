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
	"bufio"
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"strconv"

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
	mpathDeviceCount            int    = 6
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
	Lun            string            `json:"lun"`
	Iface          string            `json:"iface"`
	Chap_discovery bool              `json:"chapdiscovery"`
	Chap_session   bool              `json:"chapsession"`
	Secret         map[string]string `json:"secret"`
	InitiatorName  string            `json:"initiatorname"`
	VolName        string            `json:"volumename"`
	VolumeID       int               `json:"volumeid"`
	IsBlock        bool              `json:"isblock"`
	MpathDevice    string            `json:"mpathdevice"`
	Targets        []iscsiTarget     `json:"targets"`
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
	chap_sess = []string{
		CHAP_USERNAME,
		CHAP_PASSWORD,
		CHAP_USERNAME_IN,
		CHAP_PASSWORD_IN,
	}
	ifaceTransportNameRe = regexp.MustCompile(`iface.transport_name = (.*)\n`)
)

// Global resouce contains a sync.Mutex. Used to serialize iSCSI resource accesses.
var execCommand helper.Exec

// StatFunc stat a path, if not exists, retry maxRetries times
// when iscsi transports other than default are used
type StatFunc func(string) (os.FileInfo, error)

// GlobFunc  use glob instead as pci id of device is unknown
type GlobFunc func(string) ([]string, error)

func (iscsi *iscsistorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	const function = "NodeStageVolume"
	zlog.Debug().Msgf("%s (iscsi) called with publish context: %s", function, req.GetPublishContext())

	hostID, ports, err := validatePublishContext(req.GetPublishContext())
	if err != nil {
		e := fmt.Errorf("%s  (iscsi)- validatePublishContext - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostSecurity := req.GetPublishContext()["securityMethod"]
	useChap := req.GetVolumeContext()[common.SC_USE_CHAP]
	zlog.Debug().Msgf("%s (iscsi) - Publishing volume to host with hostID %d", function, hostID)

	initiatorName := getInitiatorName()
	if initiatorName == "" {
		e := fmt.Errorf("%s (iscsi) - iscsi initiator name not found", function)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if !strings.Contains(ports, initiatorName) {
		zlog.Debug().Msgf("%s (iscsi) - host port is not created, creating one", function)
		err = iscsi.cs.AddPortForHost(hostID, "ISCSI", initiatorName)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - AddPortForHost - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	zlog.Debug().Msgf("%s (iscsi) - setup chap auth as '%s'", function, useChap)
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
					e := fmt.Errorf("%s (iscsi) - iscsi mutual chap credentials not provided", function)
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
					e := fmt.Errorf("%s (iscsi) - iscsi mutual chap credentials not provided", function)
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
			if len(chapCreds) > 1 {
				zlog.Debug().Msgf("%s (iscsi) - create chap authentication for host %d", function, hostID)
				err := iscsi.cs.AddChapSecurityForHost(hostID, chapCreds)
				if err != nil {
					e := fmt.Errorf("%s (iscsi) - AddChapSecurityForHost - error: %s", function, err.Error())
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
		} else if hostSecurity != SECURITY_METHOD_NONE {
			zlog.Debug().Msgf("%s (iscsi) - remove chap authentication for host %d", function, hostID)
			chapCreds[SECURITY_METHOD] = SECURITY_METHOD_NONE
			err := iscsi.cs.AddChapSecurityForHost(hostID, chapCreds)
			if err != nil {
				e := fmt.Errorf("%s (iscsi) - AddChapSecurityForHost - error: %s", function, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	const function = "NodePublishVolume"
	zlog.Debug().Msgf("%s (iscsi) - volume ID %s, network_space %s mode %s readOnly %t", function, req.GetVolumeId(), req.GetVolumeContext()[common.SC_NETWORK_SPACE], req.GetVolumeCapability().GetAccessMode().Mode, req.Readonly)

	targets, err := iscsi.getISCSITargets(req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - getISCSITargets - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (iscsi) iscsi %d targets  %v", function, len(targets), targets)

	iscsiDisk, err := iscsi.getISCSIDisk(req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - getISCSIDisk - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	iscsiDisk.Targets = targets
	zlog.Debug().Msgf("%s (iscsi) - iscsiDisk: vol name %s lun %s", function, iscsiDisk.VolName, iscsiDisk.Lun)

	diskMounter, err := iscsi.getISCSIDiskMounter(iscsiDisk, req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - getISCSIDiskMounter - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - AttachDisk - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (iscsi) - iscsi attachDisk succeeded", function)

	if diskMounter.readOnly {
		zlog.Debug().Msgf("%s (iscsi) - skipping chown-chmod since this is readOnly volume", function)
	} else {
		// Chown
		err = iscsi.storageHelper.SetVolumePermissions(req)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - SetVolumePermissions - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume (iscsi) returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	const function = "NodeUnpublishVolume"
	zlog.Debug().Msgf("%s (iscsi) volume ID %s and targetPath '%s'", function, req.GetVolumeId(), req.GetTargetPath())

	err := unmountAndCleanUp(req.GetTargetPath())
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - unmountAndCleanup - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {
	const function = "NodeUnstageVolume"
	zlog.Debug().Msgf("%s (iscsi) volume ID %s", function, req.GetVolumeId())

	diskUnmounter := iscsi.getISCSIDiskUnmounter()
	stagePath := req.GetStagingTargetPath()
	var mpathDevice string

	zlog.Debug().Msgf("%s (iscsi) - staging target path: %s", function, stagePath)

	// Load iscsi disk config from json file
	dskInfo := diskInfo{
		VolumeID: diskUnmounter.iscsiDiskInfo.VolumeID,
		RootDir:  common.NODE_ROOT_DIR,
	}
	if err := loadDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		zlog.Debug().Msgf("%s (iscsi) - successfully loaded disk information from %s, mpath=[%s]", function, stagePath, mpathDevice)
	} else {
		//confFile := path.Join("/host", stagePath, diskUnmounter.iscsiDisk.VolName+".json")
		confFile := path.Join("/host", stagePath, strconv.Itoa(diskUnmounter.iscsiDiskInfo.VolumeID)+".json")
		zlog.Debug().Msgf("%s (iscsi) - check if config file exists", function)
		pathExist, pathErr := iscsi.cs.pathExists(confFile)
		if pathErr != nil {
			zlog.Error().Msgf("%s (iscsi) - pathExists - error: %s", function, pathErr.Error())
		}
		if pathErr == nil {
			if !pathExist {
				zlog.Debug().Msgf("%s (iscsi) - config file doesnt exist, calling RemoveAll with stagePath %s", function, stagePath)

				_ = debugWalkDir(stagePath)

				// TODO - Review code
				if err := os.RemoveAll(stagePath); err != nil {
					zlog.Error().Msg(err.Error())
					zlog.Warn().Msgf("%s (iscsi) - failed to RemoveAll stage path '%s': %v", function, stagePath, err)
				}
				zlog.Debug().Msgf("%s (iscsi) - removed stage path '%s'", function, stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		zlog.Warn().Msgf("%s (iscsi) - failed to get iscsi config from stage path '%s': %v", function, stagePath, err)
	}

	// remove multipath
	err = detachMpathDevice(mpathDevice, common.PROTOCOL_ISCSI)
	if err != nil {
		zlog.Warn().Msgf("%s (iscsi) - cannot detach volume with ID %s: %+v", function, req.GetVolumeId(), err)
	}

	removePath := path.Join("/host", stagePath)
	zlog.Debug().Msgf("%s (iscsi) - calling RemoveAll with removePath '%s'", function, removePath)

	_ = debugWalkDir(removePath)

	// Check if removePath is a directory or a file
	isADir, isADirError := IsDirectory(removePath)
	if isADirError != nil {
		e := fmt.Errorf("%s (iscsi) - IsDirectory - check if removePath '%s' is a directory: %v", function, removePath, isADirError)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// Remove directory contents
	if isADir {
		// removePath '/host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount'
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount/93642552.json
		// 93642552.json: {"Portals":["172.31.32.145:3260","172.31.32.146:3260","172.31.32.147:3260","172.31.32.148:3260","172.31.32.149:3260","172.31.32.150:3260"],"Iqn":"iqn.2009-11.com.infinidat:storage:infinibox-sn-1521","Iface":"172.31.32.145:3260","InitiatorName":"iqn.1994-05.com.redhat:462c9b4cda1","VolName":"93642189","MpathDevice":"/dev/dm-8"}

		zlog.Debug().Msgf("%s (iscsi) - removePath '%s' is a directory", function, removePath)
		jsonPath := fmt.Sprintf("%s/%d.json", removePath, iscsi.cs.VolProto.VolumeID)
		zlog.Debug().Msgf("%s (iscsi) - removing json file '%s'", function, jsonPath)
		if err := os.Remove(jsonPath); err != nil {
			e := fmt.Errorf("%s (iscsi) - Remove remove json file %s error: %v", function, jsonPath, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Debug().Msgf("%s (iscsi) - removePath '%s' is not a directory", function, removePath)
	}

	// Remove directory or file
	zlog.Debug().Msgf("%s (iscsi) - removing removePath '%s'", function, removePath)
	if err := os.Remove(removePath); err != nil {
		e := fmt.Errorf("%s (iscsi) - Remote - failed to remove path '%s': %v", function, removePath, err)
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
		zlog.Error().Msgf("%s (iscsi) - getMultipathDeviceCount - error: %s", function, err.Error())
	} else {
		zlog.Debug().Msgf("%s (iscsi) - multipath device count %d", function, deviceCount)
		if deviceCount == 0 {
			zlog.Debug().Msgf("%s (iscsi) - zero multipath devices - performing iscsi all sessions logout", function)
			err = logoutAllSessions()
			if err != nil {
				zlog.Error().Msgf("%s (iscsi) - iscsi logoutall error: %s", function, err.Error())
			}
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (iscsi) should never be called, called in node.go instead")
}

func (iscsi *iscsistorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (iscsi *iscsistorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	const function = "NodeExpandVolume"
	zlog.Info().Msgf("%s (iscsi) called request volume ID %s path %s\n", function, req.GetVolumeId(), req.GetVolumePath())
	response := csi.NodeExpandVolumeResponse{}

	// the block volume case
	block := req.GetVolumeCapability().GetBlock()
	zlog.Debug().Msgf("%s (iscsi) - block string is [%s]", function, block.String())

	if req.GetVolumeCapability().GetBlock() != nil {
		err := blockExpandVolume(req.GetVolumePath())
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - blockExpandVolume block volume name %s - error: %s", function, req.GetVolumePath(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		return &response, nil
	}

	// 1 - run mount | grep <volume_path> to find the multipath device name (e.g. /dev/mapper/mpathwi)
	multipathDevice, err := findMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - findMultipathDevice - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	zlog.Debug().Msgf("%s (iscsi) - command is [%s]", function, command)

	out, _, err := execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - Command %s - error: %s", function, command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	if out == "" {
		e := fmt.Errorf("%s (iscsi) - error getting multipath devices from output %s command output was empty", function, multipathDevice)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	output := strings.TrimSpace(out)
	zlog.Debug().Msgf("%s (iscsi) - output is [%s]\n", function, output)
	outputParts := strings.Split(output, "\n")
	zlog.Debug().Msgf("%s (iscsi) - lines %d\n", function, len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := range outputParts {
		if outputParts[i] != "" {
			line := strings.Split(outputParts[i], "_")
			if len(line) < 2 {
				zlog.Error().Msgf("%s (iscsi) - error getting multipath blockDevice from output %v", function, line)
				continue
			}
			blockDevice := line[1]
			zlog.Debug().Msgf("device is [%s]\n", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, _, err := execCommand.Command(command, "")
			if err != nil {
				e := fmt.Errorf("%s (iscsi) - Command %s - error writing rescan on multipath devices %s", function, command, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, e
			}
			zlog.Debug().Msgf("%s (iscsi) rescan output is [%s]\n", function, strings.TrimSpace(string(out)))
		}
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		return nil, fmt.Errorf("%s (iscsi) - error getting mpathPart from %+v", function, mpathPart)
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	zlog.Debug().Msgf("command is [%s]", command)
	out, _, err = execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s  (iscsi)- error multipathd resize map multipath devices %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]", strings.TrimSpace(string(out)))

	// 5 - run resize2fs or xfs_growfs on /dev/mapper/mpathwi
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = expandFileSystem(multipathDevice, fsType)
	if err != nil {
		e := fmt.Errorf("%s  (iscsi)- error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	return &response, nil
}

func (iscsi *iscsistorage) AttachDisk(b iscsiDiskMounter) (mntPath string, err error) {
	var devicePath string
	var iscsiTransport string
	const function = "AttachDisk"

	zlog.Debug().Msgf("%s (iscsi), fsType: %s readOnly: %v mountOpts: %v targetPath: %s stagePath: %s",
		function, b.fsType, b.readOnly, b.mountOptions, b.targetPath, b.stagePath)

	zlog.Debug().Msgf("check that provided interface '%s' is available", b.Iface)
	isToLogOutput := false
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", b.Iface), isToLogOutput)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) cannot read interface: %s output: %s error: %v", function, b.Iface, string(out), err)
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("%s (iscsi) provided interface '%s': ", function, b.Iface) //, out)

	iscsiTransport = iscsi.extractTransportName(string(out))
	zlog.Debug().Msgf("%s (iscsi) iscsiTransport: %s", function, iscsiTransport)
	if iscsiTransport == "" {
		e := fmt.Errorf("%s (iscsi) could not find transport name in iface %s", function, b.Iface) // TODO - b.Iface here realy should be newIface...or does it matter?
		zlog.Error().Msg(e.Error())
		return "", e
	}

	// If not found, create new iface and copy parameters from pre-configured (default) iface to the created iface
	// Use one interface per iSCSI network-space, i.e. usually one per IBox.
	// TODO what does a blank Initiator name mean?
	targets := b.Targets
	if b.InitiatorName == "" {
		for i := range targets {
			// Look for existing interface named newIface. Clone default iface, if not found.
			zlog.Debug().Msgf("%s (iscsi) initiatorName: %s", function, b.InitiatorName)
			newIface := targets[i].Portals[0] // Do not append ':$volume_id'
			zlog.Debug().Msgf("%s (iscsi) required iface name: '%s'", function, newIface)
			isToLogOutput := false
			_, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", newIface), isToLogOutput)
			if err != nil {
				zlog.Debug().Msgf("%s (iscsi) creating new iface (clone) and copying parameters from pre-configured iface to it", function)
				err = iscsi.cloneIface(b, newIface)
				if err != nil {
					e := fmt.Errorf("%s (iscsi) failed to clone iface: %s error: %v", function, b.Iface, err)
					zlog.Error().Msg(e.Error())
					return "", e
				}
				zlog.Debug().Msgf("%s (iscsi) new iface created '%s'", function, newIface)
			} else {
				zlog.Debug().Msgf("%s (iscsi) required iface '%s' already exists", function, newIface)
			}
		}
	} else {
		zlog.Debug().Msgf("%s (iscsi) Using existing initiator name'%s'", function, b.InitiatorName)
	}

	for i := range targets {
		for p := range targets[i].Portals {
			zlog.Debug().Msgf("%s (iscsi) Discover targets at portal '%s'", function, targets[i].Portals[p])
			// Discover all targets associated with a portal.
			_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode discoverydb --type sendtargets --portal %s --discover --op new --op delete", targets[i].Portals[p]))
			if err != nil {
				e := fmt.Errorf("%s (iscsi) failed to discover targets at portal '%s': %v ", function, out, err)
				zlog.Error().Msgf("%s", e.Error())
				return "", e
			}
		}
	}

	if !b.Chap_session {
		zlog.Debug().Msgf("%s (iscsi) target iqn: %s - Not using CHAP", function, targets[0].Iqn)
	} else {
		// Loop over portals:
		// - Set CHAP usage and update discoverydb with CHAP secret
		for i := range targets {
			for p := range targets[i].Portals {
				zlog.Debug().Msgf("%s (iscsi) target iface: %s iqn: %s - use CHAP at portal: %s", function, b.Iface, targets[i].Iqn, targets[i].Portals[p])
				err = iscsi.updateISCSINode(b, targets[i].Iqn, targets[i].Portals[p])
				if err != nil {
					zlog.Error().Msgf("%s", err.Error())
					// failure to update node db is rare. But deleting record will likely impact those who already start using it.
					zlog.Error().Msgf("%s", fmt.Errorf("%s (iscsi) : Failed to update iscsi node for portal: %s error: %v", function, targets[i].Portals[p], err.Error()))
					continue
				}
			}
		}
	}

	sessionDetails := getSessionDetails()
	zlog.Debug().Msgf("%s (iscsi) - list sessions before any logins: %v", function, sessionDetails)

	for i := range targets {
		// Check for at least one session. If none, login.
		zlog.Debug().Msgf("%s (iscsi) - list sessions to target iqn: %s", function, targets[i].Iqn)

		iqnFound := false
		for j := range sessionDetails {
			if sessionDetails[j].iqn == targets[i].Iqn {
				iqnFound = true
			}
		}
		if !iqnFound {
			for p := range targets[i].Portals {
				zlog.Debug().Msgf("%s (iscsi) - login to iscsi target iqn %s at all portals using interface %s", function, targets[i].Iqn, targets[i].Portals[p])
				_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --targetname %s --portal %s --login", targets[i].Iqn, targets[i].Portals[p]))
				if err != nil {
					zlog.Error().Msg(err.Error())
					if status.Code(err) != codes.AlreadyExists {
						e := fmt.Errorf("%s (iscsi) - iscsi login failed to target iqn: %s, portal %s err: %s", function, targets[i].Iqn, targets[i].Portals[p], err.Error())
						zlog.Error().Msg(e.Error())
						return "", e
					} else {
						zlog.Debug().Msgf("%s (iscsi) - already logged in to target iqn: %s portal %s", function, targets[i].Iqn, targets[i].Portals[p])
					}
				}
			}
		} else {
			if len(targets[i].Portals) > 0 {
				zlog.Debug().Msgf("%s (iscsi) - already logged into iscsi target iqn %s using interface %s", function, targets[i].Iqn, targets[i].Portals[0])
			} else {
				zlog.Debug().Msgf("%s (iscsi) - already logged into iscsi target iqn %s", function, targets[i].Iqn)
			}
		}

	}
	sessionDetails = getSessionDetails()
	zlog.Debug().Msgf("%s (iscsi) - list sessions after any logins: %v", function, sessionDetails)

	// Rescan for LUN b.lun
	// Find hosts. TODO - take heed of portals.
	hosts, err := getHostIDs()
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - finding hosts failed: %s", function, err)
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("%s (iscsi) - hosts [%+v] len %d", function, hosts, len(hosts))

	// For each host, scan using lun

	wwid, err := rescanDeviceMap(hosts, b.VolName, b.Lun)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - rescanDeviceMap failed for volume ID %s and lun %s: %s", function, b.VolName, b.Lun, err)
		zlog.Error().Msg(e.Error())
		return "", e
	}

	if wwid == "" {
		e := fmt.Errorf("%s (iscsi) - searchDisk rescan error wwid not found", function)
		zlog.Error().Msg(e.Error())
		return "", e
	}

	zlog.Debug().Msgf("%s (iscsi) - searchDisk sleeping 3 seconds to allow devmapper time to work wwid [%s]", function, wwid)

	tries := 10 //currently this means a max of 10 seconds which is ample

	var dm string
	for i := range tries {
		dm = getDMDevicePath(wwid)
		if dm != "" {
			zlog.Debug().Msgf("%s (iscsi) - found wwid '%s' and dm '%s'", function, wwid, dm)
			break
		}
		if dm != "" && strings.Contains(dm, "dm-") {
			zlog.Debug().Msgf("%s (iscsi) - found a valid dm device [%s] iteration %d", function, dm, i)
			break
		}
		time.Sleep(time.Second * 1)
	}
	zlog.Debug().Msgf("%s (iscsi) - found dm [%s]", function, dm)

	if dm == "" {
		return "", fmt.Errorf("%s (iscsi) - error, could not find a dm device for wwid %s", function, wwid)
	}
	trimmedDeviceName := strings.Replace(dm, "/host", "", 1)
	var thisMpath string
	thisMpath, err = findMpathFromDevice(trimmedDeviceName)
	if err != nil {
		zlog.Error().Msgf("%s (iscsi) - findMpathFromDevice for [%s] error is [%s]", function, trimmedDeviceName, err.Error())
	}
	// here trimmedDeviceName is /dev/dm-3 and thisMpath is mpathtf
	zlog.Debug().Msgf("%s (iscsi) for [%s] thisMpath is [%s]", function, trimmedDeviceName, thisMpath)

	// Make sure we use a valid devicepath to find mpio device.
	//mntPath = b.targetPath
	// Mount device

	var options []string

	config := diskInfo{
		VolumeID:    b.VolumeID,
		MpathDevice: thisMpath,
		RootDir:     common.NODE_ROOT_DIR,
	}

	devicePath = devMapperDir + thisMpath

	err = mountLogic(config, b.targetPath, devicePath, b.stagePath, b.fsType, options, b.IsBlock, b.readOnly)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) mountLogic() failed, error %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}

	zlog.Debug().Msgf("%s (iscsi) mounted volume with device path %s successfully at '%s'", function, devicePath, mntPath)
	return devicePath, nil
}

func portalMounter(portal string) string {
	if !strings.Contains(portal, ":") {
		portal = portal + ":3260"
	}
	return portal
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

func (iscsi *iscsistorage) getISCSIDisk(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	initiatorName := getInitiatorName()

	volName := strconv.Itoa(iscsi.cs.VolProto.VolumeID)
	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	zlog.Debug().Msgf("getISCSIDisk (iscsi) volume: %d context: %v publish context: %v", iscsi.cs.VolProto.VolumeID, volContext, publishContext)

	lun := publishContext["lun"]
	if lun == "" {
		return nil, fmt.Errorf("getISCSIDisk (iscsi) iscsi: LUN is missing")
	}

	useChap := volContext[common.SC_USE_CHAP]
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
		VolumeID:       iscsi.cs.VolProto.VolumeID,
		VolName:        volName,
		Lun:            lun,
		Iface:          "default",
		Chap_discovery: chapDiscovery,
		Chap_session:   chapSession,
		Secret:         secret,
		InitiatorName:  initiatorName,
	}, nil
}

func (iscsi *iscsistorage) getISCSIDiskMounter(iscsiDisk *iscsiDisk, req *csi.NodePublishVolumeRequest) (*iscsiDiskMounter, error) {
	m := &iscsiDiskMounter{
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
		m.readOnly = true
	}
	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	blockVolCapability := reqVolCapability.GetBlock()

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their iSCSI device
		iscsiDisk.IsBlock = false

		m.fsType = mountVolCapability.GetFsType()

		// mountOptions - could be nothing
		m.mountOptions = mountVolCapability.GetMountFlags()

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

	m.iscsiDisk = iscsiDisk

	return m, nil
}

func (iscsi *iscsistorage) getISCSIDiskUnmounter() *iscsiDiskUnmounter {
	return &iscsiDiskUnmounter{
		iscsiDiskInfo: &iscsiDisk{
			VolName:  strconv.Itoa(iscsi.cs.VolProto.VolumeID),
			VolumeID: iscsi.cs.VolProto.VolumeID,
		},
		mounter: mount.NewWithoutSystemd(""),
		exec:    utilexec.New(),
	}
}

func (iscsi *iscsistorage) parseSessionSecret(useChap string, secretParams map[string]string) (map[string]string, error) {
	var ok bool
	const function = "parseSessionSecret"
	secret := make(map[string]string)

	if useChap == USE_CHAP || useChap == USE_CHAP_MUTUAL {
		if len(secretParams) == 0 {
			return secret, errors.New("parseSessionSecret (iscsi): required chap secrets not provided")
		}
		if secret[CHAP_USERNAME], ok = secretParams[CHAP_USERNAME]; !ok {
			return secret, fmt.Errorf("%s (iscsi): %s not found in secret", function, CHAP_USERNAME)
		}
		if secret[CHAP_PASSWORD], ok = secretParams[CHAP_PASSWORD]; !ok {
			return secret, fmt.Errorf("%s (iscsi): %s not found in secret", function, CHAP_PASSWORD)
		}
		if useChap == USE_CHAP_MUTUAL {
			if secret[CHAP_USERNAME_IN], ok = secretParams[CHAP_USERNAME_IN]; !ok {
				return secret, fmt.Errorf("%s (iscsi): %s not found in secret", function, CHAP_USERNAME_IN)
			}
			if secret[CHAP_PASSWORD_IN], ok = secretParams[CHAP_PASSWORD_IN]; !ok {
				return secret, fmt.Errorf("%s(iscsi): %s not found in secret", function, CHAP_PASSWORD_IN)
			}
		}
		secret["SecretsType"] = USE_CHAP
	}
	return secret, nil
}

func (iscsi *iscsistorage) updateISCSINode(b iscsiDiskMounter, iqn string, portal string) error {
	const function = "updateISCSINode"
	if !b.Chap_session {
		return nil
	}

	zlog.Debug().Msgf("%s (iscsi) update node with CHAP", function)
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name node.session.auth.authmethod --value CHAP", portal, iqn))
	if err != nil {
		e := fmt.Errorf("%s (iscsi): failed to update node with CHAP, output: %v", function, string(out))
		zlog.Error().Msg(e.Error())
		return e
	}

	for _, k := range chap_sess {
		v := b.Secret[k]
		if len(v) > 0 {
			zlog.Debug().Msgf("%s (iscsi) update node session key/value", function)
			out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name %q --value %q", portal, iqn, k, v))
			if err != nil {
				e := fmt.Errorf("%s (iscsi): failed to update node session key %q with value %q error: %v", function, k, v, string(out))
				zlog.Error().Msg(e.Error())
				return e
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) extractTransportName(ifaceOutput string) (iscsiTransport string) {
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

func (iscsi *iscsistorage) parseIscsiadmShow(output string) (map[string]string, error) {
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

func (iscsi *iscsistorage) cloneIface(b iscsiDiskMounter, newIface string) error {
	var lastErr error
	const function = "cloneIface"
	zlog.Debug().Msgf("%s (iscsi) - find pre-configured iface records", function)
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", b.Iface))
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		lastErr = fmt.Errorf("%s (iscsi): failed to show iface records: %s (%v)", function, string(out), err)
		return lastErr
	}
	zlog.Debug().Msgf("%s (iscsi) - pre-configured iface records found: %s", function, out)

	// parse obtained records
	params, err := iscsi.parseIscsiadmShow(string(out))
	if err != nil {
		zlog.Error().Msgf("%s (iscsi) - parse - error: %s", function, err.Error())
		lastErr = fmt.Errorf("%s (iscsi): Failed to parse iface records: %s (%v)", function, string(out), err)
		return lastErr
	}
	// update initiatorname
	params["iface.initiatorname"] = b.InitiatorName

	zlog.Debug().Msgf("%s (iscsi) - create new interface", function)
	out, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op new", newIface))
	if err != nil {
		lastErr = fmt.Errorf("%s (iscsi): failed to create new iface: %s (%v)", function, string(out), err)
		return lastErr
	}

	// update new iface records
	for key, val := range params {
		zlog.Debug().Msgf("%s (iscsi) - update records of interface '%s'", function, newIface)
		_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op update --name %q --value %q", newIface, key, val))
		if err != nil {
			zlog.Error().Msgf("%s", err.Error())
			_, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op delete", newIface))
			if err != nil {
				lastErr = fmt.Errorf("%s (iscsi) - failed to delete iface '%s': %s ", function, newIface, err)
				return lastErr
			}

			lastErr = fmt.Errorf("%s (iscsi): failed to update iface records: %s (%v). iface(%s) will be used", function, string(out), err, b.Iface)
			break
		}
	}
	return lastErr
}

func (iscsi *iscsistorage) getISCSITargets(req *csi.NodePublishVolumeRequest) (targets []iscsiTarget, err error) {
	const function = "getISCSITargets"
	networkSpaces := strings.Split(req.GetVolumeContext()[common.SC_NETWORK_SPACE], ",")
	if len(networkSpaces) == 0 {
		return targets, fmt.Errorf("%s (iscsi) no network spaces found", function)
	}
	zlog.Debug().Msgf("networkSpaces %v", networkSpaces)
	if iscsi.cs.Api == nil {
		return targets, fmt.Errorf("%s (iscsi) no api found", function)
	}

	var portalsExist bool
	targets = make([]iscsiTarget, len(networkSpaces))

	for i := range networkSpaces {
		zlog.Debug().Msgf("%s (iscsi) - getting nspace by name: %v", function, networkSpaces[i])
		nspace, err := iscsi.cs.IboxApi.GetNetworkSpaceByName(networkSpaces[i])
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - error getting network space: %s error: %v", function, networkSpaces[i], err)
			zlog.Error().Msg(e.Error())
			return targets, status.Error(codes.InvalidArgument, e.Error())
		}
		zlog.Debug().Msgf("%s (iscsi) - got nspace by name: %s", function, nspace.Name)

		targets[i] = iscsiTarget{
			Iqn:     nspace.Properties.IscsiIqn,
			Portals: []string{},
		}
		for _, p := range nspace.Portals {
			if !p.Enabled {
				zlog.Error().Msgf("%s (iscsi) - network space %s ip address %s is disabled, not adding to list of available ip addresses", function, nspace.Name, p.IpAdress)
				continue
			}

			err := iscsi.storageHelper.ValidateIPAddress(p.IpAdress, nspace.Properties.IscsiTcpPort)
			if err != nil {
				zlog.Error().Msgf("%s (iscsi) - error getting iscsi network space %s ip connection to %s %d error: %v", function, networkSpaces[i], p.IpAdress, nspace.Properties.IscsiTcpPort, err)
				continue
			}

			zlog.Debug().Msgf("%s (iscsi) - adding iscsi network space %s ip connection to %s %d list", function, networkSpaces[i], p.IpAdress, nspace.Properties.IscsiTcpPort)
			targets[i].Portals = append(targets[i].Portals, portalMounter(p.IpAdress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, fmt.Errorf("%s (iscsi) - there are zero network space ip addresses available", function)
	}
	return targets, nil
}

func getSessionDetails() (results []SessionDetails) {
	rawOutput, _, err := execCommand.Command("iscsiadm", "--mode session")
	if err != nil {
		zlog.Error().Msgf("getISCSITargets (iscsi) - session list failed, error: %v", err)
		return results
	}
	lines, err := stringToLines(rawOutput)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return results
	}
	results = make([]SessionDetails, 0)
	for i := range lines {
		if len(lines[i]) > 0 {
			parts := strings.Split(lines[i], " ")
			protocolParts := strings.Split(parts[0], ":")
			ipaddressParts := strings.Split(parts[2], ",")
			s := SessionDetails{
				protocol:  protocolParts[0],
				ipAddress: ipaddressParts[0],
				hostID:    ipaddressParts[1],
				iqn:       parts[3],
			}
			results = append(results, s)
		}
	}

	return results
}

func stringToLines(s string) (lines []string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	err = scanner.Err()
	return
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
	for i := range parts {
		if strings.Contains(parts[i], "tcp") {
			trim := strings.TrimLeft(parts[i], " ")
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
