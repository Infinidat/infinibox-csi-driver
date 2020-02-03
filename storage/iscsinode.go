package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	iscsi_lib "github.com/kubernetes-csi/csi-lib-iscsi/iscsi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
)

var (
	chap_st = []string{
		"discovery.sendtargets.auth.username",
		"discovery.sendtargets.auth.password",
		"discovery.sendtargets.auth.username_in",
		"discovery.sendtargets.auth.password_in"}
	chap_sess = []string{
		"node.session.auth.username",
		"node.session.auth.password",
		"node.session.auth.username_in",
		"node.session.auth.password_in"}
	ifaceTransportNameRe = regexp.MustCompile(`iface.transport_name = (.*)\n`)
)

func (iscsi *iscsistorage) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest) (
	*csi.NodePublishVolumeResponse, error) {


	iscsiInfo, err := iscsi.getISCSIInfo(req)
	if err != nil {

		return nil, status.Error(codes.Internal, err.Error())
	}
	diskMounter := iscsi.getISCSIDiskMounter(iscsiInfo, req)

	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest) (
	*csi.NodeUnpublishVolumeResponse, error) {
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req)
	targetPath := req.GetTargetPath()
	err := iscsi.DetachDisk(*diskUnmounter, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) AttachDisk(b iscsiDiskMounter) (string, error) {

	log.Print("IN AttachDisk iscsiDisk---------------------------->  : ", b.iscsiDisk)
	log.Print("IN AttachDisk----------------------------> params : ", b.connector)
	log.Print("IN AttachDisk portals---------------------------->  : ", b.iscsiDisk.Portals)
	log.Print("IN AttachDisk iqn---------------------------->  : ", b.iscsiDisk.Iqn)
	log.Print("IN AttachDisk lun---------------------------->  : ", b.iscsiDisk.lun)
	log.Print("IN AttachDisk b.connector.DoDiscovery---------------------------->  : ", b.connector.DoDiscovery)
	log.Print("IN AttachDisk b.connector.DoCHAPDiscovery---------------------------->  : ", b.connector.DoCHAPDiscovery)

	devicePath, err := iscsi_lib.Connect(*b.connector)
	if err != nil {
		return "", err
	}
	if devicePath == "" {
		return "", fmt.Errorf("connect reported success, but no path returned")
	}

	// Mount device
	mntPath := b.targetPath
	notMnt, err := b.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("Heuristic determination of mount point failed:%v", err)
	}
	if !notMnt {
		log.Infof("iscsi: %s already mounted", mntPath)
		return "", nil
	}

	if err := os.MkdirAll(mntPath, 0750); err != nil {
		log.Errorf("iscsi: failed to mkdir %s, error", mntPath)
		return "", err
	}

	var options []string

	if b.readOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}
	options = append(options, b.mountOptions...)

	err = b.mounter.FormatAndMount(devicePath, mntPath, b.fsType, options)
	if err != nil {
		log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, b.fsType, mntPath, err)
		return devicePath, err
	}

	// Persist iscsi disk config to json file for DetachDisk path
	file := path.Join(mntPath, b.VolName+".json")
	err = iscsi_lib.PersistConnector(b.connector, file)
	if err != nil {
		log.Errorf("failed to persist connection info: %v", err)
		log.Errorf("disconnecting volume and failing the publish request because persistence files are required for reliable Unpublish")
		return "", fmt.Errorf("unable to create persistence file for connection")
	}

	return devicePath, err
}

func (iscsi *iscsistorage) DetachDisk(c iscsiDiskUnmounter, targetPath string) error {
	_, cnt, err := mount.GetDeviceNameFromMount(c.mounter, targetPath)
	if err != nil {
		log.Errorf("iscsi detach disk: failed to get device from mnt: %s\nError: %v", targetPath, err)
		return err
	}
	if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExists {
		log.Warningf("Warning: Unmount skipped because path does not exist: %v", targetPath)
		return nil
	}
	if err = c.mounter.Unmount(targetPath); err != nil {
		log.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}
	cnt--
	if cnt != 0 {
		return nil
	}

	// load iscsi disk config from json file
	file := path.Join(targetPath, c.iscsiDisk.VolName+".json")
	connector, err := iscsi_lib.GetConnectorFromFile(file)
	if err != nil {
		log.Errorf("iscsi detach disk: failed to get iscsi config from path %s Error: %v", targetPath, err)
		return err
	}

	iqn := ""
	portals := []string{}
	if len(connector.Targets) > 0 {
		iqn = connector.Targets[0].Iqn
		for _, t := range connector.Targets {
			portals = append(portals, t.Portal)
		}
	}

	iscsi_lib.Disconnect(iqn, portals)

	if err := os.RemoveAll(targetPath); err != nil {
		log.Errorf("iscsi: failed to remove mount path Error: %v", err)
		return err
	}

	return nil
}

func (iscsi *iscsistorage) getISCSIInfo(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	volName := req.GetVolumeContext()["Name"]
	tp := req.GetVolumeContext()["targetPortal"]
	networkSpace := req.GetVolumeContext()["networkspace"]
	nspace, err := iscsi.cs.api.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return nil, fmt.Errorf("Error getting network space")
	}
	iqn := nspace.Properties.IscsiIqn
	lun := req.GetVolumeContext()["lun"]
	portals := nspace.Portals
	secret := req.GetSecrets()

	// temp -->
	secretmanual := make(map[string]string)
	secretmanual["node.session.auth.username"] = iqn
	secretmanual["node.session.auth.password"] = secret["password"]
	secretmanual["node.session.auth.username_in"] = ""
	secretmanual["node.session.auth.password_in"] = ""

	secretmanual["node.sendtargets.auth.username"] = iqn
	secretmanual["node.sendtargets.auth.password"] = secret["password"]
	secretmanual["node.sendtargets.auth.username_in"] = ""
	secretmanual["node.sendtargets.auth.password_in"] = ""
	// temp -->

	sessionSecret, err := parseSessionSecret(secretmanual)
	if err != nil {
		return nil, err
	}
	discoverySecret, err := parseDiscoverySecret(secretmanual)
	if err != nil {
		return nil, err
	}

	portal := iscsi.portalMounter(tp)
	//	portal := portalMounter(tp)
	var bkportal []string
	bkportal = append(bkportal, portal)
	for _, portal := range portals {
		bkportal = append(bkportal, iscsi.portalMounter(portal.IpAdress))
	}
	iface := req.GetVolumeContext()["iscsiInterface"]
	initiatorName := iscsi.cs.getIscsiInitiatorName()

	doDiscovery := true
	chapDiscovery := false
	if req.GetVolumeContext()["chapAuthDiscovery"] == "true" {
		chapDiscovery = true
	}

	chapSession := false
	if req.GetVolumeContext()["chapAuthSession"] == "true" {
		chapSession = true
	}

	var lunVal int32
	if lun != "" {
		l, err := strconv.Atoi(lun)
		if err != nil {
			return nil, err
		}
		lunVal = int32(l)
	}

	return &iscsiDisk{
		VolName:         volName,
		Portals:         bkportal,
		Iqn:             iqn,
		lun:             lunVal,
		Iface:           iface,
		chapDiscovery:   chapDiscovery,
		chapSession:     chapSession,
		secret:          secret,
		sessionSecret:   sessionSecret,
		discoverySecret: discoverySecret,
		InitiatorName:   initiatorName,
		doDiscovery:     doDiscovery}, nil
}

func buildISCSIConnector(iscsiInfo *iscsiDisk) *iscsi_lib.Connector {
	targets := []iscsi_lib.TargetInfo{}
	target := iscsi_lib.TargetInfo{}
	for _, p := range iscsiInfo.Portals {
		target.Iqn = iscsiInfo.Iqn
		if strings.Contains(p, ":") {
			arr := strings.Split(p, ":")
			target.Portal = arr[0]
			target.Port = arr[1]
		} else {
			target.Portal = p
			target.Port = "3260"
		}
	}
	targets = append(targets, target)
	target.Iqn = iscsiInfo.Iqn
	target.Portal = iscsiInfo.Portals[0]
	c := iscsi_lib.Connector{
		VolumeName:       iscsiInfo.VolName,
		Targets:          targets,
		Multipath:        len(iscsiInfo.Portals) > 1,
		DoDiscovery:      iscsiInfo.doDiscovery,
		DoCHAPDiscovery:  iscsiInfo.chapDiscovery,
		DiscoverySecrets: iscsiInfo.discoverySecret,
		Lun:              iscsiInfo.lun,
	}

	if iscsiInfo.sessionSecret != (iscsi_lib.Secrets{}) {
		c.SessionSecrets = iscsiInfo.sessionSecret
		if iscsiInfo.discoverySecret != (iscsi_lib.Secrets{}) {
			c.DiscoverySecrets = iscsiInfo.discoverySecret
		}
	}

	return &c
}

func (iscsi *iscsistorage) getISCSIDiskMounter(iscsiInfo *iscsiDisk, req *csi.NodePublishVolumeRequest) *iscsiDiskMounter {
	readOnly := req.GetReadonly()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	return &iscsiDiskMounter{
		iscsiDisk:    iscsiInfo,
		fsType:       fsType,
		readOnly:     readOnly,
		mountOptions: mountOptions,
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: mount.NewOsExec()},
		exec:         mount.NewOsExec(),
		targetPath:   req.GetTargetPath(),
		deviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
		connector:    buildISCSIConnector(iscsiInfo),
	}
}

func (iscsi *iscsistorage) getISCSIDiskUnmounter(req *csi.NodeUnpublishVolumeRequest) *iscsiDiskUnmounter {
	return &iscsiDiskUnmounter{
		iscsiDisk: &iscsiDisk{
			VolName: req.GetVolumeId(),
		},
		mounter: mount.New(""),
		exec:    mount.NewOsExec(),
	}
}

func (iscsi *iscsistorage) portalMounter(portal string) string {
	if !strings.Contains(portal, ":") {
		portal = portal + ":3260"
	}
	return portal
}

func parseSecret(secretParams string) map[string]string {
	var secret map[string]string
	if err := json.Unmarshal([]byte(secretParams), &secret); err != nil {
		return nil
	}
	return secret
}

func parseSessionSecret(secretParams map[string]string) (iscsi_lib.Secrets, error) {
	var ok bool
	secret := iscsi_lib.Secrets{}

	if len(secretParams) == 0 {
		return secret, nil
	}

	if secret.UserName, ok = secretParams["node.session.auth.username"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.username not found in secret")
	}
	if secret.Password, ok = secretParams["node.session.auth.password"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.password not found in secret")
	}
	if secret.UserNameIn, ok = secretParams["node.session.auth.username_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.username_in not found in secret")
	}
	if secret.PasswordIn, ok = secretParams["node.session.auth.password_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.password_in not found in secret")
	}

	secret.SecretsType = "chap"
	return secret, nil
}

func parseDiscoverySecret(secretParams map[string]string) (iscsi_lib.Secrets, error) {
	var ok bool
	secret := iscsi_lib.Secrets{}

	if len(secretParams) == 0 {
		return secret, nil
	}

	if secret.UserName, ok = secretParams["node.sendtargets.auth.username"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.username not found in secret")
	}
	if secret.Password, ok = secretParams["node.sendtargets.auth.password"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.password not found in secret")
	}
	if secret.UserNameIn, ok = secretParams["node.sendtargets.auth.username_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.username_in not found in secret")
	}
	if secret.PasswordIn, ok = secretParams["node.sendtargets.auth.password_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.password_in not found in secret")
	}

	secret.SecretsType = "chap"
	return secret, nil
}

type iscsiDisk struct {
	Portals         []string
	Iqn             string
	lun             int32
	Iface           string
	chapDiscovery   bool
	doDiscovery     bool
	chapSession     bool
	secret          map[string]string
	sessionSecret   iscsi_lib.Secrets
	discoverySecret iscsi_lib.Secrets
	InitiatorName   string
	VolName         string
}

type iscsiDiskMounter struct {
	*iscsiDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         mount.Exec
	deviceUtil   util.DeviceUtil
	targetPath   string
	connector    *iscsi_lib.Connector
}

type iscsiDiskUnmounter struct {
	*iscsiDisk
	mounter mount.Interface
	exec    mount.Exec
}

func (iscsi *iscsistorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String()+"---  NodeStageVolume not implemented")
}
func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String()+"---  NodeUnstageVolume not implemented")
}

func (iscsi *iscsistorage) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest) (
	*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

func (iscsi *iscsistorage) NodeGetInfo(
	ctx context.Context,
	req *csi.NodeGetInfoRequest) (
	*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())

}

func (iscsi *iscsistorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}
