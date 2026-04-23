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
	"fmt"
	"log/slog"
	"runtime"
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
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

const (
	devMapperDir             string = dmsetup.DevMapperDir // ie /dev/mapper/
	CHAPInboundUsername             = "security_chap_inbound_username"
	CHAPInboundSecret               = "security_chap_inbound_secret"
	CHAPOutboundUsername            = "security_chap_outbound_username"
	CHAPOutboundSecret              = "security_chap_outbound_secret"
	SecurityMethod                  = "security_method"
	SecurityMethodNONE              = "NONE"
	SecurityMethodCHAP              = "CHAP"
	SecurityMethodMutualCHAP        = "MUTUAL_CHAP"
)

type iscsiDiskUnmounter struct {
	iscsiDiskInfo *iscsiDevice
	mounter       mount.Interface
	exec          utilexec.Interface // mount.Exec
}

type iscsiTarget struct {
	Portals []string
	Iqn     string
}

type iscsiDevice struct {
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
	UseCHAP           = "chap"
	UseMutualCHAP     = "mutual_chap"
	ISCSITransportTCP = "tcp"
	CHAPUsername      = "node.session.auth.username"
	CHAPPassword      = "node.session.auth.password"
	CHAPUsernameIn    = "node.session.auth.username_in"
	CHAPPasswordIn    = "node.session.auth.password_in"
)

var (
	CHAPSessionCredentials = []string{
		CHAPUsername,
		CHAPPassword,
		CHAPUsernameIn,
		CHAPPasswordIn,
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
	slog.Debug("start", "publish context", req.GetPublishContext(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), iscsi.CS.IboxAPI))

	hostID, ports, err := storagecommon.ValidatePublishContext(req.GetPublishContext())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	slog.Debug("publishing volume to host", "hostID", hostID)

	initiatorName, err := getInitiatorName()
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	if !strings.Contains(ports, initiatorName) {
		slog.Debug("host port is not created, creating one")
		err = iscsi.CS.AddPortForHost(ctx, hostID, "ISCSI", initiatorName)
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	useChap := req.GetVolumeContext()[common.StorageClassUseCHAP]
	slog.Debug("setup chap", "auth", useChap)
	hostSecurity := req.GetPublishContext()["securityMethod"]
	if strings.ToLower(hostSecurity) != useChap || !strings.Contains(ports, initiatorName) {
		err := addCHAPSecurity(ctx, iscsi, useChap, hostID, req)
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	slog.Debug("start", "volume id", req.GetVolumeId(), "network space", req.GetVolumeContext()[common.StorageClassNetworkSpace], "access mode", req.GetVolumeCapability().GetAccessMode().Mode, "readonly", req.Readonly,
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), iscsi.CS.IboxAPI))

	targets, err := iscsi.getISCSITargets(ctx, req)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("iscsi targets", "count", len(targets), "targets", targets)

	iscsiDisk, err := iscsi.getISCSIDisk(req)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	iscsiDisk.Targets = targets
	slog.Debug("iscsiDisk", "volume name", iscsiDisk.VolName, "lun", iscsiDisk.Lun)

	//diskMounter, err := iscsi.getISCSIDiskMounter(iscsiDisk, req)
	diskMounter, err := storagecommon.GetDiskMounter(req)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	_, err = iscsi.AttachDisk(*diskMounter, iscsiDisk)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("iscsi attachDisk succeeded")

	if diskMounter.ReadOnly {
		slog.Debug("skipping chown-chmod since this is readOnly volume")
	} else {
		// Chown
		err = iscsi.StorageHelper.SetVolumePermissions(req)
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume (iscsi) returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	slog.Debug("start", "volume id", req.GetVolumeId(), "target path", req.GetTargetPath())

	err := storagecommon.UnmountAndCleanUp(req.GetTargetPath())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *ISCSIstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {
	slog.Debug("start", "volume ID", req.GetVolumeId())

	diskUnmounter := iscsi.getISCSIDiskUnmounter()
	stagePath := req.GetStagingTargetPath()
	var mpathDevice string

	slog.Debug("info", "staging target path", stagePath)

	// Load iscsi disk config from json file
	dskInfo := storagecommon.DiskInfo{
		VolumeID: diskUnmounter.iscsiDiskInfo.VolumeID,
		RootDir:  common.NodeRootDir,
	}
	if err := storagecommon.LoadDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		slog.Debug("successfully loaded disk information", "stagePath", stagePath, "mpathDevice", mpathDevice)
	} else {
		// confFile := path.Join("/host", stagePath, diskUnmounter.iscsiDisk.VolName+".json")
		confFile := path.Join("/host", stagePath, strconv.Itoa(diskUnmounter.iscsiDiskInfo.VolumeID)+".json")
		slog.Debug("check if config file exists", "path", confFile)
		pathExist, pathErr := iscsi.CS.PathExists(confFile)
		if pathErr != nil {
			slog.Error("pathExists", "error", pathErr.Error())
		}
		if pathErr == nil {
			if !pathExist {
				slog.Debug("config file doesnt exist, calling RemoveAll", "stagePath", stagePath)

				_ = storagecommon.DebugWalkDir(stagePath)

				if err := os.RemoveAll(stagePath); err != nil {
					slog.Error(err.Error())
					slog.Warn("failed to RemoveAll", "stage path", stagePath, "error", err)
				}
				slog.Debug("removed stage path", "path", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		slog.Warn("failed to get iscsi config from stage path", "path", stagePath, "error", err)
	}

	// remove multipath
	err = storagecommon.DetachMpathDevice(mpathDevice, common.ProtocolISCSI)
	if err != nil {
		slog.Warn("cannot detach volume", "ID", req.GetVolumeId(), "error", err)
	}

	removePath := path.Join("/host", stagePath)
	slog.Debug("calling RemoveAll", "removePath", removePath)

	_ = storagecommon.DebugWalkDir(removePath)

	// Check if removePath is a directory or a file
	isADir, isADirError := storagecommon.IsDirectory(removePath)
	if isADirError != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, isADirError.Error()),
		}
		return nil, e
	}

	// Remove directory contents
	if isADir {
		// removePath '/host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount'
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount
		// Found path /host/var/lib/kubelet/plugins/kubernetes.io/csi/pv/csi-6e48953803/globalmount/93642552.json
		// 93642552.json: {"Portals":["172.31.32.145:3260","172.31.32.146:3260","172.31.32.147:3260","172.31.32.148:3260","172.31.32.149:3260","172.31.32.150:3260"],"Iqn":"iqn.2009-11.com.infinidat:storage:infinibox-sn-1521","Iface":"172.31.32.145:3260","InitiatorName":"iqn.1994-05.com.redhat:462c9b4cda1","VolName":"93642189","MpathDevice":"/dev/dm-8"}

		slog.Debug("removePath is a directory", "path", removePath)
		jsonPath := fmt.Sprintf("%s/%d.json", removePath, iscsi.CS.VolProto.VolumeID)
		slog.Debug("removing json file", "file", jsonPath)
		if err := os.Remove(jsonPath); err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	} else {
		slog.Debug("removePath is not a directory", "path", removePath)
	}

	// Remove directory or file
	slog.Debug("removing removePath", "path", removePath)
	if err := os.Remove(removePath); err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	// logout all iscsid sessions if there are zero devices, this stops iscid from
	// maintaining tcp connections to the ibox when there are zero devices

	// start by waiting a small amount of time to avoid a race condition with multipathd as
	// it takes it a bit to actually remove any devices we are checking against
	time.Sleep(time.Second * 3)

	deviceCount, err := getMultipathDeviceCount()
	if err != nil {
		slog.Error("getMultipathDeviceCount", "error", err.Error())
	} else {
		slog.Debug("multipath", "device count", deviceCount)
		if deviceCount == 0 {
			slog.Debug("zero multipath devices - performing iscsi all sessions logout")
			err = logoutAllSessions()
			if err != nil {
				slog.Error("(iscsi) - iscsi logoutall", "error", err.Error())
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
	slog.Info("start", "request volume ID", req.GetVolumeId(), "path", req.GetVolumePath(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), iscsi.CS.IboxAPI))
	response := csi.NodeExpandVolumeResponse{}

	// the block volume case
	block := req.GetVolumeCapability().GetBlock()
	slog.Debug("info", "block string", block.String())

	if req.GetVolumeCapability().GetBlock() != nil {
		err := storagecommon.BlockExpandVolume(req.GetVolumePath())
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
		return &response, nil
	}

	// 1 - run mount | grep <volume_path> to find the multipath device name (e.g. /dev/mapper/mpathwi)
	multipathDevice, err := storagecommon.FindMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	slog.Debug("command", "command", command)

	out, _, err := execCommand.Command(command, "")
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	if out == "" {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("error getting multipath devices from output %s command output was empty", multipathDevice)),
		}
		return nil, e
	}

	output := strings.TrimSpace(out)
	slog.Debug("info", "output", output)
	outputParts := strings.Split(output, "\n")
	slog.Debug("info", "lines", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for index := range outputParts {
		if outputParts[index] != "" {
			line := strings.Split(outputParts[index], "_")
			if len(line) < 2 {
				slog.Error("error getting multipath blockDevice from output", "line", line)
				continue
			}
			blockDevice := line[1]
			slog.Debug("info", "device", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, _, err := execCommand.Command(command, "")
			if err != nil {
				_, file, line, _ := runtime.Caller(0)
				e := storagecommon.ImplementationError{
					Code: int(codes.Internal),
					Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
				}
				return nil, e
			}
			slog.Debug("rescan output", "output", strings.TrimSpace(out))
		}
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	slog.Debug("executing", "command", command)
	out, _, err = execCommand.Command(command, "")
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("multipathd resize map", "output", strings.TrimSpace(out))

	// 5 - run resize2fs or xfs_growfs on /dev/mapper/mpathwi
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = storagecommon.ExpandFileSystem(multipathDevice, fsType)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	return &response, nil
}

func (iscsi *ISCSIstorage) AttachDisk(diskMounter storagecommon.Mounter, iscsiDisk *iscsiDevice) (mountPath string, err error) {
	var devicePath string

	slog.Debug("attach disk", "provided interface", iscsiDisk.Iface, "fsType", diskMounter.FsType, "readOnly", diskMounter.ReadOnly, "mountOpts", diskMounter.MountOptions, "targetPath", diskMounter.TargetPath, "stagePath", diskMounter.StagePath)

	_, err = iscsi.extractTransportName(iscsiDisk.Iface)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}

	// If not found, create new iface and copy parameters from pre-configured (default) iface to the created iface
	// Use one interface per iSCSI network-space, i.e. usually one per IBox.
	targets := iscsiDisk.Targets
	if iscsiDisk.InitiatorName == "" {
		for _, target := range targets {
			// Look for existing interface named newIface. Clone default iface, if not found.
			newIface := target.Portals[0] // Do not append ':$volume_id'
			slog.Debug("info", "initiatorName", iscsiDisk.InitiatorName, "required iface name", newIface)
			isToLogOutput := false
			commandOutput, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", newIface), isToLogOutput)
			if err != nil {
				slog.Debug("creating new iface (clone) and copying parameters from pre-configured iface to it", "commandOutput", commandOutput)
				err = iscsi.cloneIface(*iscsiDisk, newIface)
				if err != nil {
					_, file, line, _ := runtime.Caller(0)
					e := storagecommon.ImplementationError{
						Code: int(codes.Internal),
						Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
					}
					return "", e
				}
				slog.Debug("new iface created", "interface", newIface)
			} else {
				slog.Debug("required iface already exists", "interface", newIface)
			}
		}
	} else {
		slog.Debug("Using existing initiator name", "name", iscsiDisk.InitiatorName)
	}

	err = discoverISCITargets(targets)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}

	iscsi.updateForCHAP(*iscsiDisk, targets)

	err = loginToTargets(targets)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}

	// Rescan for LUN b.lun
	hosts, err := getHostIDs()
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}

	// For each host, scan using lun
	wwid, err := storagecommon.RescanDeviceMap(hosts, iscsiDisk.VolName, iscsiDisk.Lun)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}

	slog.Debug("searchDisk sleeping 3 seconds to allow devmapper time to work", "wwid", wwid)

	tries := 10 // currently this means a max of 10 seconds which is ample

	var dmDevice string
	for sleepIteration := range tries {
		dmDevice = storagecommon.GetDMDevicePath(wwid)
		if dmDevice != "" {
			slog.Debug("found wwid and dm", "wwid", wwid, "dm", dmDevice)
			break
		}
		if dmDevice != "" && strings.Contains(dmDevice, "dm-") {
			slog.Debug("found a valid dm device", "dm", dmDevice, "iteration", sleepIteration)
			break
		}
		time.Sleep(time.Second * 1)
	}
	slog.Debug("found dm", "dm", dmDevice)

	if dmDevice == "" {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("error, could not find a dm device for wwid: %s", wwid)),
		}
		return "", e
	}
	trimmedDeviceName := strings.Replace(dmDevice, "/host", "", 1)
	var thisMpath string
	thisMpath, err = storagecommon.FindMpathFromDevice(trimmedDeviceName)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}
	// here trimmedDeviceName is /dev/dm-3 and thisMpath is mpathtf
	slog.Debug("info", "device", trimmedDeviceName, "mpath", thisMpath)

	// Make sure we use a valid devicepath to find mpio device.
	// mntPath = b.targetPath
	// Mount device

	var options []string

	config := storagecommon.DiskInfo{
		VolumeID:    iscsiDisk.VolumeID,
		MpathDevice: thisMpath,
		RootDir:     common.NodeRootDir,
	}

	devicePath = devMapperDir + thisMpath

	err = storagecommon.MountLogic(config, diskMounter.TargetPath, devicePath, diskMounter.StagePath, diskMounter.FsType, options, diskMounter.IsBlock, diskMounter.ReadOnly)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}

	slog.Debug("mounted volume with device path successfully", "devicepath", devicePath, "mountpath", mountPath)
	return devicePath, nil
}

func getInitiatorName() (string, error) {
	cmd := "cat /etc/iscsi/initiatorname.iscsi | grep InitiatorName="
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		slog.Error("failed to get initiator name. Is iSCSI initiator installed", "error", err)
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return "", e
	}
	initiatorName := string(out)
	initiatorName = strings.TrimSuffix(initiatorName, "\n")
	slog.Debug("info", "host initiator name", initiatorName)
	arr := strings.Split(initiatorName, "=")
	if arr[1] == "" {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, "initiator name is empty"),
		}
		return "", e
	}
	return arr[1], nil
}

func (iscsi *ISCSIstorage) getISCSIDisk(req *csi.NodePublishVolumeRequest) (*iscsiDevice, error) {
	initiatorName, err := getInitiatorName()
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	volName := strconv.Itoa(iscsi.CS.VolProto.VolumeID)
	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	slog.Debug("start", "volume id", iscsi.CS.VolProto.VolumeID, "volumeContext", volContext, "publishContext", publishContext)

	lun := publishContext["lun"]
	if lun == "" {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, "LUN is missing"),
		}
		return nil, e
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
	if chapSession {
		secret, err = iscsi.parseSessionSecret(useChap, secret)
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	return &iscsiDevice{
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

func (iscsi *ISCSIstorage) getISCSIDiskUnmounter() *iscsiDiskUnmounter {
	return &iscsiDiskUnmounter{
		iscsiDiskInfo: &iscsiDevice{
			VolName:  strconv.Itoa(iscsi.CS.VolProto.VolumeID),
			VolumeID: iscsi.CS.VolProto.VolumeID,
		},
		mounter: mount.NewWithoutSystemd(""),
		exec:    utilexec.New(),
	}
}

func (iscsi *ISCSIstorage) parseSessionSecret(useChap string, secretParams map[string]string) (map[string]string, error) {
	var valid bool
	secret := make(map[string]string)

	if useChap == UseCHAP || useChap == UseMutualCHAP {
		if len(secretParams) == 0 {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, "parseSessionSecret (iscsi): required chap secrets not provided"),
			}
			return secret, e
		}
		if secret[CHAPUsername], valid = secretParams[CHAPUsername]; !valid {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("%s not found in secret", CHAPUsername)),
			}
			return secret, e
		}
		if secret[CHAPPassword], valid = secretParams[CHAPPassword]; !valid {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("%s not found in secret", CHAPPassword)),
			}
			return secret, e
		}
		if useChap == UseMutualCHAP {
			if secret[CHAPUsernameIn], valid = secretParams[CHAPUsernameIn]; !valid {
				_, file, line, _ := runtime.Caller(0)
				e := storagecommon.ImplementationError{
					Code: int(codes.Internal),
					Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("%s not found in secret", CHAPUsernameIn)),
				}
				return secret, e
			}
			if secret[CHAPPasswordIn], valid = secretParams[CHAPPasswordIn]; !valid {
				_, file, line, _ := runtime.Caller(0)
				e := storagecommon.ImplementationError{
					Code: int(codes.Internal),
					Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("%s not found in secret", CHAPPasswordIn)),
				}
				return secret, e
			}
		}
		secret["SecretsType"] = UseCHAP
	}
	return secret, nil
}

func (iscsi *ISCSIstorage) updateISCSINode(iscsiDisk iscsiDevice, iqn string, portal string) error {
	if !iscsiDisk.CHAPSession {
		return nil
	}

	slog.Debug("update node with CHAP")
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name node.session.auth.authmethod --value CHAP", portal, iqn))
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()+out),
		}
		return e
	}

	for _, credential := range CHAPSessionCredentials {
		v := iscsiDisk.Secret[credential]
		if len(v) > 0 {
			slog.Debug("update node session key/value")
			out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --portal %s --targetname %s --op update --name %q --value %q", portal, iqn, credential, v))
			if err != nil {
				_, file, line, _ := runtime.Caller(0)
				e := storagecommon.ImplementationError{
					Code: int(codes.Internal),
					Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()+out),
				}
				return e
			}
		}
	}
	return nil
}

func (iscsi *ISCSIstorage) extractTransportName(diskMounterIface string) (iscsiTransport string, err error) {
	slog.Debug("check that provided interface is available", "interface", diskMounterIface)
	isToLogOutput := false
	commandOutput, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", diskMounterIface), isToLogOutput)
	if err != nil {
		return "", common.Errorf("%w", err)
	}
	rexOutput := ifaceTransportNameRe.FindStringSubmatch(commandOutput)
	if rexOutput == nil {
		return "", nil
	}
	iscsiTransport = rexOutput[1]

	// While iface.transport_name is a required parameter, handle it being unspecified anyways
	if iscsiTransport == "<empty>" {
		iscsiTransport = ISCSITransportTCP
	}
	if iscsiTransport == "" {
		return "", common.Errorf("could not find transport name in iface: %s", diskMounterIface)
	}
	slog.Debug("info", "iscsiTransport", iscsiTransport)
	return iscsiTransport, nil
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
			return nil, common.Errorf("invalid iface setting %v", iface)
		}
		// iscsi_ifacename is immutable once the iface is created
		if iface[0] == "iface.iscsi_ifacename" {
			continue
		}
		params[iface[0]] = iface[2]
	}
	return params, nil
}

func (iscsi *ISCSIstorage) cloneIface(disk iscsiDevice, newIface string) error {
	var lastErr error
	slog.Debug("find pre-configured iface records")
	out, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op show", disk.Iface))
	if err != nil {
		return common.Errorf("%w", err)
	}
	slog.Debug("pre-configured iface records found", "output", out)

	// parse obtained records
	params, err := iscsi.parseIscsiadmShow(out)
	if err != nil {
		return common.Errorf("%w", err)
	}
	// update initiatorname
	params["iface.initiatorname"] = disk.InitiatorName

	slog.Debug("create new interface")
	out, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op new", newIface))
	if err != nil {
		return common.Errorf("%w", err)
	}

	// update new iface records
	// TODO evaluate this block of code for correctness
	for key, val := range params {
		slog.Debug("update records", "interface", newIface, "key", key, "value", val)
		_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op update --name %q --value %q", newIface, key, val))
		if err != nil {
			slog.Error(err.Error())
			_, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode iface --interface %s --op delete", newIface))
			if err != nil {
				return common.Errorf("%w", err)
			}

			lastErr = common.Errorf("failed to update iface records: %s error: %s iface: %s will be used", out, err, disk.Iface)
			break
		}
	}
	return lastErr
}

func (iscsi *ISCSIstorage) getISCSITargets(ctx context.Context, req *csi.NodePublishVolumeRequest) (targets []iscsiTarget, err error) {
	networkSpaces := strings.Split(req.GetVolumeContext()[common.StorageClassNetworkSpace], ",")
	if len(networkSpaces) == 0 {
		return targets, common.Errorf("no network spaces found")
	}
	slog.Debug("info", "networkSpaces", networkSpaces)
	if iscsi.CS.API == nil {
		return targets, common.Errorf("no api found")
	}

	var portalsExist bool
	targets = make([]iscsiTarget, len(networkSpaces))

	for index, networkSpace := range networkSpaces {
		slog.Debug("getting nspace by name", "networkspace", networkSpace)
		thisNetworkSpace, err := iscsi.CS.IboxAPI.GetNetworkSpaceByName(ctx, networkSpace)
		if err != nil {
			return targets, common.Errorf("%w", err)
		}
		slog.Debug("got nspace by name", "networkspace", thisNetworkSpace.Name)

		targets[index] = iscsiTarget{
			Iqn:     thisNetworkSpace.Properties.ISCSIIqn,
			Portals: []string{},
		}
		for _, portal := range thisNetworkSpace.Portals {
			if !portal.Enabled {
				slog.Error("network space is disabled, not adding to list of available ip addresses", "networkspace", thisNetworkSpace.Name, "ipaddress", portal.IPAddress)
				continue
			}

			err := iscsi.StorageHelper.ValidateIPAddress(portal.IPAddress, thisNetworkSpace.Properties.ISCSITCPPort)
			if err != nil {
				slog.Error("error getting iscsi network space", "networkspace", networkSpace, "ipaddress", portal.IPAddress, "port", thisNetworkSpace.Properties.ISCSITCPPort, "error", err)
				continue
			}

			slog.Debug("adding iscsi network space ip connection to list", "networkspace", networkSpace, "ipaddress", portal.IPAddress, "port", thisNetworkSpace.Properties.ISCSITCPPort)
			targets[index].Portals = append(targets[index].Portals, storagecommon.PortalMounter(portal.IPAddress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, common.Errorf("no network space IP addresses available")
	}
	return targets, nil
}

func getSessionDetails() (results []SessionDetails) {
	rawOutput, _, err := execCommand.Command("iscsiadm", "--mode session")
	if err != nil {
		slog.Error("session list failed", "error", err)
		return results
	}
	lines, err := storagecommon.StringToLines(rawOutput)
	if err != nil {
		slog.Error("error getting StringToLines", "error", err)
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
		return deviceCount, common.Errorf("%w", err)
	}
	devices := strings.Fields(string(out))
	slog.Debug("info", "multipath output", string(out))
	return len(devices), nil
}

func logoutAllSessions() (err error) {
	command := "iscsiadm --mode node --logoutall=all"
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)

	var out []byte
	out, err = exec.Command("bash", "-c", pipefailCmd).CombinedOutput()
	if err != nil {
		return common.Errorf("%w", err)
	}
	slog.Debug("isciadm logoutall", "output", string(out))
	return nil
}

func getHostIDs() (hosts []string, err error) {
	rawOutput, _, err := execCommand.Command("iscsiadm", "-m host -P0")
	if err != nil {
		return hosts, common.Errorf("%w", err)
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
				return hosts, common.Errorf("error could not parse host number %s", trim)
			}
			replaced := strings.ReplaceAll(rawNumber[1], "[", "")
			hostID := strings.ReplaceAll(replaced, "]", "")
			hosts = append(hosts, hostID)
		}
	}

	slog.Debug("info", "hosts", hosts, "number of hosts", len(hosts))
	return hosts, nil
}

func addChapSecurityForHost(ctx context.Context, cs storagecommon.Commonservice, hostID int, credentials map[string]string) error {
	_, err := cs.IboxAPI.AddHostSecurity(ctx, credentials, hostID)
	if err != nil {
		return common.Errorf("%w", err)
	}
	return nil
}

func (iscsi *ISCSIstorage) updateForCHAP(iscsiDisk iscsiDevice, targets []iscsiTarget) {
	if !iscsiDisk.CHAPSession {
		slog.Debug("target iqn - Not using CHAP", "iqn", targets[0].Iqn)
	} else {
		// Loop over portals:
		// - Set CHAP usage and update discoverydb with CHAP secret
		for index := range targets {
			for portalIndex := range targets[index].Portals {
				slog.Debug("target iface- use CHAP at portal", "iface", iscsiDisk.Iface, "iqn", targets[index].Iqn, "portal", targets[index].Portals[portalIndex])
				err := iscsi.updateISCSINode(iscsiDisk, targets[index].Iqn, targets[index].Portals[portalIndex])
				if err != nil {
					slog.Error(err.Error())
					// failure to update node db is rare. But deleting record will likely impact those who already start using it.
					slog.Error("Failed to update iscsi node", "portal", targets[index].Portals[portalIndex], "error", err.Error())
					continue
				}
			}
		}
	}
}

func discoverISCITargets(targets []iscsiTarget) (err error) {
	for _, target := range targets {
		for portalIndex := range target.Portals {
			slog.Debug("discover targets at portal", "portal", target.Portals[portalIndex])
			// Discover all targets associated with a portal.
			commandOutput, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("--mode discoverydb --type sendtargets --portal %s --discover --op new --op delete", target.Portals[portalIndex]))
			if err != nil {
				return common.Errorf("command output %s %w", commandOutput, err)
			}
		}
	}
	return nil
}

func loginToTargets(targets []iscsiTarget) (err error) {
	sessionDetails := getSessionDetails()
	slog.Debug("list sessions before any logins", "details", sessionDetails)

	for index := range targets {
		// Check for at least one session. If none, login.
		slog.Debug("list sessions to target iqn", "iqn", targets[index].Iqn)

		iqnFound := false
		for j := range sessionDetails {
			if sessionDetails[j].iqn == targets[index].Iqn {
				iqnFound = true
			}
		}
		if !iqnFound {
			for portal := range targets[index].Portals {
				slog.Debug("login to iscsi target iqn at all portals using interface", "iqn", targets[index].Iqn, "portal", targets[index].Portals[portal])
				_, _, err = execCommand.Command("iscsiadm", fmt.Sprintf("--mode node --targetname %s --portal %s --login", targets[index].Iqn, targets[index].Portals[portal]))
				if err != nil {
					slog.Error(err.Error())
					if status.Code(err) != codes.AlreadyExists {
						return common.Errorf("iscsi login failed to target iqn: %s, portal %s err: %s", targets[index].Iqn, targets[index].Portals[portal], err.Error())
					} else {
						slog.Debug("already logged in to target", "iqn", targets[index].Iqn, "portal", targets[index].Portals[portal])
					}
				}
			}
		} else {
			if len(targets[index].Portals) > 0 {
				slog.Debug("already logged into iscsi target iqn using interface", "iqn", targets[index].Iqn, "portal", targets[index].Portals[0])
			} else {
				slog.Debug("already logged into iscsi target iqn", "iqn", targets[index].Iqn)
			}
		}
	}
	sessionDetails = getSessionDetails()
	slog.Debug("list sessions after any logins", "sessions", sessionDetails)
	return nil
}

func addCHAPSecurity(ctx context.Context, iscsi *ISCSIstorage, useChap string, hostID int, req *csi.NodeStageVolumeRequest) (err error) {
	secrets := req.GetSecrets()
	chapCreds := make(map[string]string)

	if useChap == UseCHAP || useChap == UseMutualCHAP {
		if secrets[CHAPUsername] != "" && secrets[CHAPPassword] != "" {
			chapCreds[CHAPInboundUsername] = secrets[CHAPUsername]
			chapCreds[CHAPInboundSecret] = secrets[CHAPPassword]
			chapCreds[SecurityMethod] = SecurityMethodCHAP
		} else {
			return common.Errorf("iscsi mutual chap credentials not provided")
		}
	}
	if useChap == UseMutualCHAP {
		if secrets[CHAPUsernameIn] != "" && secrets[CHAPPasswordIn] != "" && chapCreds[SecurityMethod] == SecurityMethodCHAP {
			chapCreds[CHAPOutboundUsername] = secrets[CHAPUsernameIn]
			chapCreds[CHAPOutboundSecret] = secrets[CHAPPasswordIn]
			chapCreds[SecurityMethod] = SecurityMethodMutualCHAP
		} else {
			return common.Errorf("iscsi mutual chap credentials not provided")
		}
	}
	if len(chapCreds) > 1 {
		slog.Debug("create chap authentication", "host", hostID)
		err := addChapSecurityForHost(ctx, iscsi.CS, hostID, chapCreds)
		if err != nil {
			return common.Errorf("%w", err)
		}
	}
	return nil
}
