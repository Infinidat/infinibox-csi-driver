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
	"encoding/json"
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
	*iscsiDisk
	mounter mount.Interface
	exec    utilexec.Interface // mount.Exec
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
	lun            string
	Iface          string
	chap_discovery bool
	chap_session   bool
	secret         map[string]string
	InitiatorName  string
	VolName        string
	isBlock        bool
	MpathDevice    string
	Targets        []iscsiTarget
}

type SessionDetails struct {
	protocol  string
	ipAddress string
	hostID    string
	iqn       string
}

const (
	USE_CHAP = "chap"
	//USE_CHAP_MUTUAL     = "chap_mutual"
	USE_CHAP_MUTUAL     = "mutual_chap"
	ISCSI_TCP_TRANSPORT = "tcp"
	CHAP_USERNAME       = "node.session.auth.username"
	CHAP_PASSWORD       = "node.session.auth.password"
	CHAP_USERNAME_IN    = "node.session.auth.username_in"
	CHAP_PASSWORD_IN    = "node.session.auth.password_in"
)

var (
	// chap_st = []string{
	// 	"discovery.sendtargets.auth.username",
	// 	"discovery.sendtargets.auth.password",
	// 	"discovery.sendtargets.auth.username_in",
	// 	"discovery.sendtargets.auth.password_in",
	// }
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
	zlog.Debug().Msgf("NodeStageVolume called with publish context: %s", req.GetPublishContext())

	hostID, ports, err := validatePublishContext(req.GetPublishContext())
	if err != nil {
		zlog.Error().Msgf("NodeStageVolume - validatePublishContext - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	hostSecurity := req.GetPublishContext()["securityMethod"]
	useChap := req.GetVolumeContext()[common.SC_USE_CHAP]
	zlog.Debug().Msgf("Publishing volume to host with hostID %d", hostID)

	initiatorName := getInitiatorName()
	if initiatorName == "" {
		e := fmt.Errorf("NodeStageVolume - iscsi initiator name not found")
		zlog.Error().Msgf("%s", e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if !strings.Contains(ports, initiatorName) {
		zlog.Debug().Msgf("host port is not created, creating one")
		err = iscsi.cs.AddPortForHost(hostID, "ISCSI", initiatorName)
		if err != nil {
			zlog.Error().Msgf("NodeStageVolume - AddPortForHost - error: %s", err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	zlog.Debug().Msgf("setup chap auth as '%s'", useChap)
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
					e := fmt.Errorf("NodeStageVolume - iscsi mutual chap credentials not provided")
					zlog.Error().Msgf("%s", e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
			if useChap == USE_CHAP_MUTUAL {
				if secrets[CHAP_USERNAME_IN] != "" && secrets[CHAP_PASSWORD_IN] != "" && chapCreds[SECURITY_METHOD] == SECURITY_METHOD_CHAP {
					chapCreds[CHAP_OUTBOUND_USERNAME] = secrets[CHAP_USERNAME_IN]
					chapCreds[CHAP_OUTBOUND_SECRET] = secrets[CHAP_PASSWORD_IN]
					chapCreds[SECURITY_METHOD] = SECURITY_METHOD_MUTUAL_CHAP
				} else {
					e := fmt.Errorf("NodeStageVolume - iscsi mutual chap credentials not provided")
					zlog.Error().Msgf("%s", e.Error())
					return nil, status.Error(codes.Internal, e.Error())
				}
			}
			if len(chapCreds) > 1 {
				zlog.Debug().Msgf("create chap authentication for host %d", hostID)
				err := iscsi.cs.AddChapSecurityForHost(hostID, chapCreds)
				if err != nil {
					zlog.Error().Msgf("NodeStageVolume - AddChapSecurityForHost - error: %s", err.Error())
					return nil, status.Error(codes.Internal, err.Error())
				}
			}
		} else if hostSecurity != SECURITY_METHOD_NONE {
			zlog.Debug().Msgf("remove chap authentication for host %d", hostID)
			chapCreds[SECURITY_METHOD] = SECURITY_METHOD_NONE
			err := iscsi.cs.AddChapSecurityForHost(hostID, chapCreds)
			if err != nil {
				zlog.Error().Msgf("NodeStageVolume - AddChapSecurityForHost - error: %s", err.Error())
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	zlog.Debug().Msgf("NodePublishVolume volume ID %s, network_space %s mode %s readOnly %t", req.GetVolumeId(), req.GetVolumeContext()[common.SC_NETWORK_SPACE], req.GetVolumeCapability().GetAccessMode().Mode, req.Readonly)

	targets, err := iscsi.getISCSITargets(req)
	if err != nil {
		zlog.Error().Msgf("NodePublishVolume - getISCSITargets - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	zlog.Debug().Msgf("NodePublishVolume iscsi %d targets  %v", len(targets), targets)

	iscsiDisk, err := iscsi.getISCSIDisk(req)
	if err != nil {
		zlog.Error().Msgf("NodePublishVolume - getISCSIDisk - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	iscsiDisk.Targets = targets
	zlog.Debug().Msgf("iscsiDisk: vol name %s lun %s", iscsiDisk.VolName, iscsiDisk.lun)

	diskMounter, err := iscsi.getISCSIDiskMounter(iscsiDisk, req)
	if err != nil {
		zlog.Error().Msgf("NodePublishVolume - getISCSIDiskMounter - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		zlog.Error().Msgf("NodePublishVolume - AttachDisk - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	zlog.Debug().Msgf("iscsi attachDisk succeeded")

	if diskMounter.readOnly {
		zlog.Debug().Msgf("skipping chown-chmod since this is readOnly volume")
	} else {
		// Chown
		err = iscsi.storageHelper.SetVolumePermissions(req)
		if err != nil {
			zlog.Error().Msgf("NodePublishVolume - SetVolumePermissions - error: %s", err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	zlog.Debug().Msgf("NodeUnpublishVolume volume ID %s and targetPath '%s'", volumeId, targetPath)

	err := unmountAndCleanUp(targetPath)
	if err != nil {
		zlog.Error().Msgf("NodeUnpublishVolume - unmountAndCleanup - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {

	zlog.Debug().Msgf("NodeUnstageVolume volume ID %s", req.GetVolumeId())

	diskUnmounter := iscsi.getISCSIDiskUnmounter()
	stagePath := req.GetStagingTargetPath()
	var mpathDevice string

	zlog.Debug().Msgf("staging target path: %s", stagePath)

	// Load iscsi disk config from json file
	if err := iscsi.loadDiskInfoFromFile(diskUnmounter.iscsiDisk, stagePath); err == nil {
		zlog.Debug().Msgf("successfully loaded disk information from %s", stagePath)
		mpathDevice = diskUnmounter.iscsiDisk.MpathDevice
	} else {
		confFile := path.Join("/host", stagePath, diskUnmounter.iscsiDisk.VolName+".json")
		zlog.Debug().Msgf("check if config file exists")
		pathExist, pathErr := iscsi.cs.pathExists(confFile)
		if pathErr != nil {
			zlog.Error().Msgf("NodeUnstageVolume - pathExists - error: %s", pathErr.Error())
		}
		if pathErr == nil {
			if !pathExist {
				zlog.Debug().Msgf("config file does not exist")
				zlog.Debug().Msgf("calling RemoveAll with stagePath %s", stagePath)

				_ = debugWalkDir(stagePath)

				// TODO - Review code
				if err := os.RemoveAll(stagePath); err != nil {
					zlog.Error().Msgf("%s", err.Error())
					zlog.Warn().Msgf("failed to RemoveAll stage path '%s': %v", stagePath, err)
				}
				zlog.Debug().Msgf("removed stage path '%s'", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		zlog.Warn().Msgf("failed to get iscsi config from stage path '%s': %v", stagePath, err)
	}

	// remove multipath
	err = detachMpathDevice(mpathDevice, common.PROTOCOL_ISCSI)
	if err != nil {
		zlog.Warn().Msgf("NodeUnstageVolume cannot detach volume with ID %s: %+v", req.GetVolumeId(), err)
	}

	removePath := path.Join("/host", stagePath)
	zlog.Debug().Msgf("calling RemoveAll with removePath '%s'", removePath)

	_ = debugWalkDir(removePath)

	// Check if removePath is a directory or a file
	isADir, isADirError := IsDirectory(removePath)
	if isADirError != nil {
		err := fmt.Errorf("NodeUnstageVolume - IsDirectory - check if removePath '%s' is a directory: %v", removePath, isADirError)
		zlog.Error().Msgf("%s", err.Error())
		return nil, err
	}

	// Remove directory contents
	if isADir {
		// removePath '/host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount'
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount/93642552.json
		// 93642552.json: {"Portals":["172.31.32.145:3260","172.31.32.146:3260","172.31.32.147:3260","172.31.32.148:3260","172.31.32.149:3260","172.31.32.150:3260"],"Iqn":"iqn.2009-11.com.infinidat:storage:infinibox-sn-1521","Iface":"172.31.32.145:3260","InitiatorName":"iqn.1994-05.com.redhat:462c9b4cda1","VolName":"93642189","MpathDevice":"/dev/dm-8"}

		zlog.Debug().Msgf("removePath '%s' is a directory", removePath)
		jsonPath := fmt.Sprintf("%s/%d.json", removePath, iscsi.cs.VolProto.VolumeID)
		zlog.Debug().Msgf("removing json file '%s'", jsonPath)
		if err := os.Remove(jsonPath); err != nil {
			zlog.Error().Msgf("NodeUnstageVolume - Remove remove json file %s error: %v", jsonPath, err)
			return nil, err
		}
	} else {
		zlog.Debug().Msgf("removePath '%s' is not a directory", removePath)
	}

	// Remove directory or file
	zlog.Debug().Msgf("removing removePath '%s'", removePath)
	if err := os.Remove(removePath); err != nil {
		zlog.Error().Msgf("NodeUnstageVolume - Remote - failed to remove path '%s': %v", removePath, err)
		return nil, err
	}

	// logout all iscsid sessions if there are zero devices, this stops iscid from
	// maintaining tcp connections to the ibox when there are zero devices

	// start by waiting a small amount of time to avoid a race condition with multipathd as
	// it takes it a bit to actually remove any devices we are checking against
	time.Sleep(time.Second * 3)

	deviceCount, err := getMultipathDeviceCount()
	if err != nil {
		zlog.Error().Msgf("NodeUnstageVolume - getMultipathDeviceCount - error: %s", err.Error())
	} else {
		zlog.Debug().Msgf("multipath device count %d", deviceCount)
		if deviceCount == 0 {
			zlog.Debug().Msg("zero multipath devices - performing iscsi all sessions logout")
			err = logoutAllSessions()
			if err != nil {
				zlog.Error().Msgf("NodeUnstageVolume - iscsi logoutall error: %s", err.Error())
			}
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "iscsi NodeGetCapabilities should never be called, called in node.go instead")
}

func (iscsi *iscsistorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (iscsi *iscsistorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	zlog.Info().Msgf("iscsi NodeExpandVolume called request volume ID %s path %s\n", req.GetVolumeId(), req.GetVolumePath())
	response := csi.NodeExpandVolumeResponse{}

	// the block volume case
	block := req.GetVolumeCapability().GetBlock()
	zlog.Debug().Msgf("block string is [%s]", block.String())

	if req.GetVolumeCapability().GetBlock() != nil {
		err := blockExpandVolume(req.GetVolumePath())
		if err != nil {
			zlog.Error().Msgf("NodeExpandVolume - blockExpandVolume block volume name %s - error: %s", req.GetVolumePath(), err.Error())
			return nil, err
		}
		return &response, nil
	}

	// 1 - run mount | grep <volume_path> to find the multipath device name (e.g. /dev/mapper/mpathwi)
	multipathDevice, err := findMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		zlog.Error().Msgf("NodeExpandVolume - findMultipathDevice - error: %s", err.Error())
		return nil, err
	}

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	zlog.Debug().Msgf("command is [%s]", command)

	out, err := execCommand.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("NodeExpandVolume - Command %s - error: %s", command, err.Error())
		return nil, err
	}

	if out == "" {
		err := fmt.Errorf("NodeExpandVolume - error getting multipath devices from output %s command output was empty", multipathDevice)
		zlog.Error().Msg(err.Error())
		return nil, err
	}

	output := strings.TrimSpace(out)
	zlog.Debug().Msgf("output is [%s]\n", output)
	outputParts := strings.Split(output, "\n")
	zlog.Debug().Msgf("lines %d\n", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := 0; i < len(outputParts); i++ {
		if outputParts[i] != "" {
			line := strings.Split(outputParts[i], "_")
			if len(line) < 2 {
				zlog.Error().Msgf("NodeExpandVolume - error getting multipath blockDevice from output %v", line)
				continue
			}
			blockDevice := line[1]
			zlog.Debug().Msgf("device is [%s]\n", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, err := execCommand.Command(command, "")
			if err != nil {
				zlog.Error().Msgf("NodeExpandVolume - Command %s - error writing rescan on multipath devices %s", command, err.Error())
				return nil, err
			}
			zlog.Debug().Msgf("rescan output is [%s]\n", strings.TrimSpace(string(out)))
		}
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		return nil, fmt.Errorf("NodeExpandVolume - error getting mpathPart from %+v", mpathPart)
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	zlog.Debug().Msgf("command is [%s]", command)
	out, err = execCommand.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("NodeExpandVolume - error multipathd resize map multipath devices %s", err.Error())
		return nil, err
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]", strings.TrimSpace(string(out)))

	// 5 - run resize2fs /dev/mapper/mpathwi, this appears to work for both FC and iSCSI
	time.Sleep(time.Second * 5)
	command = fmt.Sprintf("resize2fs %s", multipathDevice)
	out, err = execCommand.Command(command, "")
	zlog.Debug().Msgf("command is [%s]", command)
	if err != nil {
		zlog.Error().Msgf("NodeExpandVolume - Command %s - error: %s", command, err.Error())
		return nil, err
	}
	zlog.Debug().Msgf("resize2fs output is [%s]\n", strings.TrimSpace(string(out)))

	return &response, nil
}

func (iscsi *iscsistorage) AttachDisk(b iscsiDiskMounter) (mntPath string, err error) {
	var devicePath string
	var iscsiTransport string

	zlog.Debug().Msgf("AttachDisk, disk: %v fsType: %s readOnly: %v mountOpts: %v targetPath: %s stagePath: %s",
		b.iscsiDisk, b.fsType, b.readOnly, b.mountOptions, b.targetPath, b.stagePath)

	zlog.Debug().Msgf("check that provided interface '%s' is available", b.Iface)
	isToLogOutput := false
	out, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", b.Iface), isToLogOutput)
	if err != nil {
		e := fmt.Errorf("cannot read interface: %s output: %s error: %v", b.Iface, string(out), err)
		zlog.Error().Msgf("%s", e.Error())
		return "", e
	}
	zlog.Debug().Msgf("provided interface '%s': ", b.Iface) //, out)

	iscsiTransport = iscsi.extractTransportName(string(out))
	zlog.Debug().Msgf("iscsiTransport: %s", iscsiTransport)
	if iscsiTransport == "" {
		e := fmt.Errorf("could not find transport name in iface %s", b.Iface) // TODO - b.Iface here realy should be newIface...or does it matter?
		zlog.Error().Msgf("%s", e.Error())
		return "", e
	}

	// If not found, create new iface and copy parameters from pre-configured (default) iface to the created iface
	// Use one interface per iSCSI network-space, i.e. usually one per IBox.
	// TODO what does a blank Initiator name mean?
	targets := b.Targets
	if b.InitiatorName == "" {
		for i := 0; i < len(targets); i++ {
			// Look for existing interface named newIface. Clone default iface, if not found.
			zlog.Debug().Msgf("initiatorName: %s", b.InitiatorName)
			newIface := targets[i].Portals[0] // Do not append ':$volume_id'
			zlog.Debug().Msgf("required iface name: '%s'", newIface)
			isToLogOutput := false
			_, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", newIface), isToLogOutput)
			if err != nil {
				zlog.Debug().Msgf("creating new iface (clone) and copying parameters from pre-configured iface to it")
				err = iscsi.cloneIface(b, newIface)
				if err != nil {
					zlog.Error().Msgf("failed to clone iface: %s error: %v", b.Iface, err)
					return "", err
				}
				zlog.Debug().Msgf("new iface created '%s'", newIface)
			} else {
				zlog.Debug().Msgf("required iface '%s' already exists", newIface)
			}
			// update iface name. TODO - Seems bad form to change a func param.
			//b.Iface = newIface
		}
	} else {
		zlog.Debug().Msgf("Using existing initiator name'%s'", b.InitiatorName)
	}

	for i := 0; i < len(targets); i++ {
		for p := 0; p < len(targets[i].Portals); p++ {
			zlog.Debug().Msgf("Discover targets at portal '%s'", targets[i].Portals[p])
			// Discover all targets associated with a portal.
			_, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode discoverydb --type sendtargets --portal %s --discover --op new --op delete", targets[i].Portals[p]))
			if err != nil {
				e := fmt.Errorf("failed to discover targets at portal '%s': %v ", out, err)
				zlog.Error().Msgf("%s", e.Error())
				return "", e
			}
		}
	}

	if !b.chap_session {
		zlog.Debug().Msgf("target iqn: %s - Not using CHAP", targets[0].Iqn)
	} else {
		// Loop over portals:
		// - Set CHAP usage and update discoverydb with CHAP secret
		for i := 0; i < len(targets); i++ {
			for p := 0; p < len(targets[i].Portals); p++ {
				zlog.Debug().Msgf("target iface: %s iqn: %s - use CHAP at portal: %s", b.Iface, targets[i].Iqn, targets[i].Portals[p])
				err = iscsi.updateISCSINode(b, targets[i].Iqn, targets[i].Portals[p])
				if err != nil {
					zlog.Error().Msgf("%s", err.Error())
					// failure to update node db is rare. But deleting record will likely impact those who already start using it.
					zlog.Error().Msgf("%s", fmt.Errorf("iscsi: Failed to update iscsi node for portal: %s error: %v", targets[i].Portals[p], err.Error()))
					continue
				}
			}
		}
	}

	sessionDetails := getSessionDetails()
	zlog.Debug().Msgf("list sessions before any logins: %v", sessionDetails)

	for i := 0; i < len(targets); i++ {
		// Check for at least one session. If none, login.
		zlog.Debug().Msgf("list sessions to target iqn: %s", targets[i].Iqn)

		iqnFound := false
		for j := 0; j < len(sessionDetails); j++ {
			if sessionDetails[j].iqn == targets[i].Iqn {
				iqnFound = true
			}
		}
		if !iqnFound {
			for p := 0; p < len(targets[i].Portals); p++ {
				zlog.Debug().Msgf("login to iscsi target iqn %s at all portals using interface %s", targets[i].Iqn, targets[i].Portals[p])
				_, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --targetname %s --portal %s --login", targets[i].Iqn, targets[i].Portals[p]))
				if err != nil {
					zlog.Error().Msgf("%s", err.Error())
					if status.Code(err) != codes.AlreadyExists {
						msg := fmt.Sprintf("iscsi login failed to target iqn: %s, portal %s err: %s", targets[i].Iqn, targets[i].Portals[p], err.Error())
						zlog.Error().Msg(msg)
						return "", err
					} else {
						zlog.Debug().Msgf("already logged in to target iqn: %s portal %s", targets[i].Iqn, targets[i].Portals[p])
					}
				}
			}
		} else {
			if len(targets[i].Portals) > 0 {
				zlog.Debug().Msgf("already logged into iscsi target iqn %s using interface %s", targets[i].Iqn, targets[i].Portals[0])
			} else {
				zlog.Debug().Msgf("already logged into iscsi target iqn %s", targets[i].Iqn)
			}
		}

	}
	sessionDetails = getSessionDetails()
	zlog.Debug().Msgf("list sessions after any logins: %v", sessionDetails)

	// Rescan for LUN b.lun
	// Find hosts. TODO - take heed of portals.
	hosts, err := getHostIDs()
	if err != nil {
		zlog.Error().Msgf("finding hosts failed: %s", err)
		return "", err
	}
	zlog.Debug().Msgf("hosts [%+v] len %d", hosts, len(hosts))

	// For each host, scan using lun

	wwid, err := rescanDeviceMap(hosts, b.VolName, b.lun)
	if err != nil {
		zlog.Error().Msgf("rescanDeviceMap failed for volume ID %s and lun %s: %s", b.VolName, b.lun, err)
		return "", err
	}

	if wwid == "" {
		zlog.Error().Msgf("searchDisk rescan error wwid not found")
		return "", fmt.Errorf("wwid not found")
	}

	zlog.Debug().Msgf("searchDisk sleeping 3 seconds to allow devmapper time to work wwid [%s]", wwid)

	tries := 10 //currently this means a max of 10 seconds which is ample

	var dm string
	for i := 0; i < tries; i++ {
		dm = getDMDevicePath(wwid)
		if dm != "" {
			zlog.Debug().Msgf("found wwid '%s' and dm '%s'", wwid, dm)
			break
		}
		if dm != "" && strings.Contains(dm, "dm-") {
			zlog.Debug().Msgf("found a valid dm device [%s] iteration %d", dm, i)
			break
		}
		time.Sleep(time.Second * 1)
	}
	zlog.Debug().Msgf("found dm [%s]", dm)

	if dm == "" {
		return "", fmt.Errorf("error, could not find a dm device for wwid %s", wwid)
	}
	trimmedDeviceName := strings.Replace(dm, "/host", "", 1)
	var thisMpath string
	thisMpath, err = findMpathFromDevice(trimmedDeviceName)
	if err != nil {
		zlog.Error().Msgf("findMpathFromDevice for [%s] error is [%s]", trimmedDeviceName, err.Error())
	}
	// here trimmedDeviceName is /dev/dm-3 and thisMpath is mpathtf
	zlog.Debug().Msgf("AttachDisk iscsi for [%s] thisMpath is [%s]", trimmedDeviceName, thisMpath)

	// Make sure we use a valid devicepath to find mpio device.
	mntPath = b.targetPath
	// Mount device
	notMnt, err := b.mounter.IsLikelyNotMountPoint(mntPath)
	if err == nil {
		if !notMnt {
			zlog.Debug().Msgf("%s already mounted", mntPath)
			return "", nil
		}
	} else if !os.IsNotExist(err) {
		zlog.Error().Msgf("%s", err.Error())
		return "", status.Errorf(codes.Internal, "%s exists but IsLikelyNotMountPoint failed: %v", mntPath, err)
	}

	devicePath = trimmedDeviceName
	b.iscsiDisk.MpathDevice = trimmedDeviceName

	mountOptionMode := "rw"
	mode := "0750" //rwx
	var options []string
	if b.readOnly {
		mountOptionMode = "ro"
		mode = "0550" //read only
		zlog.Debug().Msgf("readOnly so setting mountPoint to %s", mode)
	}

	if b.isBlock {
		// A block volume is a volume that will appear as a block device inside the container.
		zlog.Debug().Msgf("mounting raw block volume at given path %s", mntPath)

		zlog.Debug().Msgf("run: mkdir --parents --mode %s '%s' ", mode, filepath.Dir(mntPath))
		// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
		// MkdirAll() will cause hard-to-grok mount errors.
		cmd := exec.Command("mkdir", "--parents", "--mode", mode, filepath.Dir(mntPath))
		err = cmd.Run()
		if err != nil {
			zlog.Error().Msgf("failed to mkdir '%s': %s", mntPath, err)
			return "", err
		}

		_, err = os.Create("/host/" + mntPath)
		if err != nil {
			e := fmt.Errorf("failed to create target file %q: %v", mntPath, err)
			zlog.Error().Msgf("%s", e.Error())
			return "", e
		}

		devicePath = strings.Replace(devicePath, "/host", "", 1)

		// TODO: validate this further, see CSIC-341
		options = append(options, "bind")
		options = append(options, mountOptionMode)

		if err := b.mounter.Mount(devicePath, mntPath, "", options); err != nil {
			zlog.Error().Msgf("failed to bind mount iscsi block volume %s [%s] to %s, error %v", devicePath, b.fsType, mntPath, err)
			return "", err
		}
		if err := iscsi.createISCSIConfigFile(*(b.iscsiDisk), b.stagePath); err != nil {
			zlog.Error().Msgf("failed to save iscsi config with error: %v", err)
			return "", err
		}
		zlog.Debug().Msgf("block volume bind mounted successfully to %s", mntPath)
		return devicePath, nil
	} else {
		// A mounted (file) volume is volume that will be mounted using a specified file system
		// and appear as a directory inside the container.
		mountPoint := mntPath

		// devicePath at this point is /dev/dm-N
		zlog.Debug().Msgf("mounting volume %s with filesystem at given path %s", devicePath, mountPoint)

		devicePath = devMapperDir + thisMpath

		// Create mountPoint if it does not exist.
		_, err := os.Stat(mountPoint)
		if os.IsNotExist(err) {
			zlog.Debug().Msgf("mount point does not exist. creating mount point.")
			// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
			// MkdirAll() will cause hard-to-grok mount errors.
			_, err := execCommand.Command("mkdir", fmt.Sprintf("--parents --mode %s '%s'", mode, mountPoint))
			if err != nil {
				zlog.Error().Msgf("failed to mkdir '%s': %v", mountPoint, err)
				return "", err
			}
		} else {
			zlog.Debug().Msgf("mkdir of mountPoint not required. '%s' already exists", mountPoint)
		}

		options = append(options, mountOptionMode) // BUG: what if user separately specified "rw" option?
		options = append(options, b.mountOptions...)

		// Persist here so that even if mount fails, the globalmount metadata json
		// file will contain an mpath to use during clean up.
		zlog.Debug().Msgf("persist iscsi disk config to json file for later use, when detaching the disk")
		if err = iscsi.createISCSIConfigFile(*(b.iscsiDisk), b.stagePath); err != nil {
			zlog.Error().Msgf("failed to save iscsi config with error: %v", err)
			return "", err
		}

		if b.fsType == "xfs" {
			zlog.Debug().Msgf("device %s is of type XFS. Mounting without regard to its XFS UUID.", devicePath)
			options = append(options, "nouuid")
		}

		zlog.Debug().Msgf("format '%s' (if needed) and mount volume", devicePath)
		err = b.mounter.FormatAndMount(devicePath, mountPoint, b.fsType, options)
		zlog.Debug().Msgf("formatAndMount returned: %+v", err)
		if err != nil {
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)
			zlog.Debug().Msgf("search error for matches to handle: %+v", err)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				zlog.Error().Msgf("device %s is already mounted on %s", devicePath, mountPoint)
			} else {
				zlog.Error().Msgf("failed to mount iscsi volume %s [%s] to %s, error %+v", devicePath, b.fsType, mountPoint, err)
				_, _ = mountPathExists(mountPoint)
				return "", err
			}
		}
	}
	zlog.Debug().Msgf("mounted volume with device path %s successfully at '%s'", devicePath, mntPath)
	return devicePath, nil
}

func mountPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		zlog.Debug().Msgf("mountPathExists: Path %s exists", path)
		return true, nil
	}
	if os.IsNotExist(err) {
		zlog.Debug().Msgf("mountPathExists: Path %s does not exist", path)
		return false, nil
	}
	return false, err
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
		zlog.Error().Msgf("failed to get initiator name. Is iSCSI initiator installed? Error: %v", err)
		return ""
	}
	initiatorName := string(out)
	initiatorName = strings.TrimSuffix(initiatorName, "\n")
	zlog.Debug().Msgf("host initiator name %s ", initiatorName)
	arr := strings.Split(initiatorName, "=")
	return arr[1]
}

func (iscsi *iscsistorage) getISCSIDisk(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	initiatorName := getInitiatorName()

	volName := strconv.Itoa(iscsi.cs.VolProto.VolumeID)
	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	zlog.Debug().Msgf("volume: %d context: %v publish context: %v", iscsi.cs.VolProto.VolumeID, volContext, publishContext)

	lun := publishContext["lun"]
	if lun == "" {
		return nil, fmt.Errorf("iscsi: LUN is missing")
	}

	useChap := volContext[common.SC_USE_CHAP]
	chapSession := false
	if useChap != "none" {
		chapSession = true
	}
	chapDiscovery := false
	if volContext["discoveryCHAPAuth"] == "true" {
		chapDiscovery = true
	}
	secret := req.GetSecrets()
	var err error
	if chapSession {
		secret, err = iscsi.parseSessionSecret(useChap, secret)
		if err != nil {
			zlog.Error().Msgf("%s", err.Error())
			return nil, err
		}
	}

	return &iscsiDisk{
		VolName:        volName,
		lun:            lun,
		Iface:          "default",
		chap_discovery: chapDiscovery,
		chap_session:   chapSession,
		secret:         secret,
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
		iscsiDisk.isBlock = false

		m.fsType = mountVolCapability.GetFsType()

		// mountOptions - could be nothing
		m.mountOptions = mountVolCapability.GetMountFlags()

		// TODO: other validations needed for file?
		// - something about read-only access?
		// - check that fstype is supported?
		// - check that mount options are valid for fstype provided

	} else if mountVolCapability == nil && blockVolCapability != nil {
		// option B. user wants block access to their iSCSI device
		iscsiDisk.isBlock = true

		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			zlog.Warn().Msg("MULTI_NODE_MULTI_WRITER AccessMode requested for raw block volume, could be dangerous")
		}
		// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?

		// don't need to look at FsType or MountFlags here, only relevant for mountVol
		// TODO: other validations needed for block?
		// - something about read-only access?
	} else {
		errMsg := "Bad VolumeCapability parameters: both block and mount modes, for volume: " + req.GetVolumeId()
		zlog.Error().Msg(errMsg)
		return nil, status.Error(codes.InvalidArgument, errMsg)
	}

	m.iscsiDisk = iscsiDisk

	return m, nil
}

func (iscsi *iscsistorage) getISCSIDiskUnmounter() *iscsiDiskUnmounter {
	return &iscsiDiskUnmounter{
		iscsiDisk: &iscsiDisk{
			VolName: strconv.Itoa(iscsi.cs.VolProto.VolumeID),
		},
		mounter: mount.NewWithoutSystemd(""),
		exec:    utilexec.New(),
	}
}

func (iscsi *iscsistorage) parseSessionSecret(useChap string, secretParams map[string]string) (map[string]string, error) {
	var ok bool
	secret := make(map[string]string)

	if useChap == USE_CHAP || useChap == USE_CHAP_MUTUAL {
		if len(secretParams) == 0 {
			return secret, errors.New("iscsi: required chap secrets not provided")
		}
		if secret[CHAP_USERNAME], ok = secretParams[CHAP_USERNAME]; !ok {
			return secret, fmt.Errorf("iscsi: %s not found in secret", CHAP_USERNAME)
		}
		if secret[CHAP_PASSWORD], ok = secretParams[CHAP_PASSWORD]; !ok {
			return secret, fmt.Errorf("iscsi: %s not found in secret", CHAP_PASSWORD)
		}
		if useChap == USE_CHAP_MUTUAL {
			if secret[CHAP_USERNAME_IN], ok = secretParams[CHAP_USERNAME_IN]; !ok {
				return secret, fmt.Errorf("iscsi: %s not found in secret", CHAP_USERNAME_IN)
			}
			if secret[CHAP_PASSWORD_IN], ok = secretParams[CHAP_PASSWORD_IN]; !ok {
				return secret, fmt.Errorf("iscsi: %s not found in secret", CHAP_PASSWORD_IN)
			}
		}
		secret["SecretsType"] = USE_CHAP
	}
	return secret, nil
}

func (iscsi *iscsistorage) updateISCSINode(b iscsiDiskMounter, iqn string, portal string) error {
	if !b.chap_session {
		return nil
	}

	zlog.Debug().Msgf("update node with CHAP")
	out, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name node.session.auth.authmethod --value CHAP", portal, iqn))
	if err != nil {
		return fmt.Errorf("iscsi: failed to update node with CHAP, output: %v", string(out))
	}

	for _, k := range chap_sess {
		v := b.secret[k]
		if len(v) > 0 {
			zlog.Debug().Msgf("update node session key/value")
			out, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name %q --value %q", portal, iqn, k, v))
			if err != nil {
				return fmt.Errorf("iscsi: failed to update node session key %q with value %q error: %v", k, v, string(out))
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) createISCSIConfigFile(conf iscsiDisk, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	zlog.Debug().Msgf("creating iscsi config file at path %s", file)
	fp, err := os.Create(file)
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		return err
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		zlog.Error().Msgf("%s", err.Error())
		return err
	}
	return nil
}

func (iscsi *iscsistorage) loadDiskInfoFromFile(conf *iscsiDisk, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Open(file)
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		return fmt.Errorf("iscsi: Open %s err %s", file, err)
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		zlog.Error().Msgf("%s", err.Error())
		return fmt.Errorf("iscsi: Decode err: %v ", err)
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
			e := fmt.Errorf("iscsi invalid iface setting %v", iface)
			zlog.Error().Msgf("%s", e.Error())
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
	zlog.Debug().Msgf("find pre-configured iface records")
	out, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", b.Iface))
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		lastErr = fmt.Errorf("iscsi: failed to show iface records: %s (%v)", string(out), err)
		return lastErr
	}
	zlog.Debug().Msgf("pre-configured iface records found: %s", out)

	// parse obtained records
	params, err := iscsi.parseIscsiadmShow(string(out))
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		lastErr = fmt.Errorf("iscsi: Failed to parse iface records: %s (%v)", string(out), err)
		return lastErr
	}
	// update initiatorname
	params["iface.initiatorname"] = b.InitiatorName

	zlog.Debug().Msgf("create new interface")
	out, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op new", newIface))
	if err != nil {
		lastErr = fmt.Errorf("iscsi: failed to create new iface: %s (%v)", string(out), err)
		return lastErr
	}

	// update new iface records
	for key, val := range params {
		zlog.Debug().Msgf("update records of interface '%s'", newIface)
		_, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op update --name %q --value %q", newIface, key, val))
		if err != nil {
			zlog.Error().Msgf("%s", err.Error())
			_, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op delete", newIface))
			if err != nil {
				lastErr = fmt.Errorf("failed to delete iface '%s': %s ", newIface, err)
				return lastErr
			}

			lastErr = fmt.Errorf("iscsi: failed to update iface records: %s (%v). iface(%s) will be used", string(out), err, b.Iface)
			break
		}
	}
	return lastErr
}

// Flush a multipath device map for device.

func multipathFlush(mpath string) {
	zlog.Debug().Msgf("Running multipath -f '%s'", mpath)

	isToLogOutput := true
	if out, err := execCommand.Command("multipath", fmt.Sprintf("-f %s", mpath), isToLogOutput); err != nil {
		zlog.Error().Msgf("multipath -f '%s' failed - ignored: %s", mpath, err)
	} else {
		zlog.Debug().Msgf("multipath -f '%s' succeeded: %s", mpath, out)
	}

	// _, _ = execScsi.Command("ls", "-l /host/dev/mapper/*; echo", isToLogOutput)
	// _, _ = execScsi.Command("ls", "/host/dev/sd*; echo", isToLogOutput)
}

// Given a device like '/dev/dm-0', find its matching multipath name such as 'mpathab'.
func findMpathFromDevice(device string) (mpath string, err error) {
	deviceName := strings.Replace(device, "/dev/", "", 1)
	//command := fmt.Sprintf("multipath -l | grep --word-regexp %s | awk '{print $1}'", deviceName)
	wildcards := "\"%n_%d_\""
	command := fmt.Sprintf("multipathd show maps raw format %s | grep %s", wildcards, deviceName)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	zlog.Debug().Msgf("command [%s]", command)

	out, err := exec.Command("bash", "-c", pipefailCmd).CombinedOutput()
	if err != nil {
		e := fmt.Errorf("cannot findMpathFromDevice: %s, Error: %s: %v", device, mpath, err)
		zlog.Error().Msgf("%s", e.Error())
		return mpath, e
	}
	outParts := strings.Split(string(out), "_")
	if len(outParts) < 1 {
		e := fmt.Errorf("cannot correctly parse findMpathFromDevice: %s, out: %s", device, outParts)
		zlog.Error().Msgf("%s", e.Error())
		return mpath, e
	}
	if len(outParts) > 0 {
		mpath = outParts[0]
	}

	zlog.Debug().Msgf("device %s corresponds to multipath %s", device, mpath)
	return
}

func (iscsi *iscsistorage) getISCSITargets(req *csi.NodePublishVolumeRequest) (targets []iscsiTarget, err error) {
	networkSpaces := strings.Split(req.GetVolumeContext()[common.SC_NETWORK_SPACE], ",")
	if len(networkSpaces) == 0 {
		return targets, fmt.Errorf("no network spaces found")
	}
	zlog.Debug().Msgf("networkSpaces %v", networkSpaces)
	if iscsi.cs.Api == nil {
		return targets, fmt.Errorf("no api found")
	}

	var portalsExist bool
	targets = make([]iscsiTarget, len(networkSpaces))

	for i := 0; i < len(networkSpaces); i++ {
		zlog.Debug().Msgf("getting nspace by name: %v", networkSpaces[i])
		nspace, err := iscsi.cs.Api.GetNetworkSpaceByName(networkSpaces[i])
		if err != nil {
			e := fmt.Errorf("error getting network space: %s error: %v", networkSpaces[i], err)
			zlog.Error().Msgf("%s", e.Error())
			return targets, status.Error(codes.InvalidArgument, e.Error())
		}
		zlog.Debug().Msgf("got nspace by name: %s", nspace.Name)

		targets[i] = iscsiTarget{
			Iqn:     nspace.Properties.IscsiIqn,
			Portals: []string{},
		}
		for _, p := range nspace.Portals {
			if !p.Enabled {
				zlog.Error().Msgf("network space %s ip address %s is disabled, not adding to list of available ip addresses", nspace.Name, p.IpAdress)
				continue
			}

			err := iscsi.storageHelper.ValidateIPAddress(p.IpAdress, nspace.Properties.IscsiTcpPort)
			if err != nil {
				zlog.Error().Msgf("error getting iscsi network space %s ip connection to %s %d error: %v", networkSpaces[i], p.IpAdress, nspace.Properties.IscsiTcpPort, err)
				continue
			}

			zlog.Debug().Msgf("adding iscsi network space %s ip connection to %s %d list", networkSpaces[i], p.IpAdress, nspace.Properties.IscsiTcpPort)
			targets[i].Portals = append(targets[i].Portals, portalMounter(p.IpAdress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, fmt.Errorf("there are zero network space ip addresses available")
	}
	return targets, nil
}

func getSessionDetails() (results []SessionDetails) {
	rawOutput, err := execCommand.Command("iscsiadm", "--mode session")
	if err != nil {
		zlog.Error().Msgf("session list failed, err: %v", err)
		return results
	}
	lines, err := stringToLines(rawOutput)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return results
	}
	results = make([]SessionDetails, 0)
	for i := 0; i < len(lines); i++ {
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
		e := fmt.Errorf("multipath command error: %s", err)
		return deviceCount, e
	}
	devices := strings.Fields(string(out))
	zlog.Debug().Msgf("multipath output %s", string(out))
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
	zlog.Debug().Msgf("isciadm logoutall output %s", string(out))
	return nil
}

func getHostIDs() (hosts []string, err error) {
	rawOutput, err := execCommand.Command("iscsiadm", "-m host -P0")
	if err != nil {
		zlog.Error().Msgf("finding hosts failed: %s", err)
		return hosts, err
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
	for i := 0; i < len(parts); i++ {
		if strings.Contains(parts[i], "tcp") {
			trim := strings.TrimLeft(parts[i], " ")
			rawNumber := strings.Split(trim, " ")
			if len(rawNumber) < 2 {
				err = fmt.Errorf("error, could not parse host number [%s]", trim)
				return hosts, err
			}
			replaced := strings.ReplaceAll(rawNumber[1], "[", "")
			hostID := strings.ReplaceAll(replaced, "]", "")
			hosts = append(hosts, hostID)
		}
	}

	return hosts, nil
}
