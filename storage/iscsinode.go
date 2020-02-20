package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
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

func (iscsi *iscsistorage) NodePublishVolume(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest) (
	*csi.NodePublishVolumeResponse, error) {
	log.Print("IN NodePublishVolume req.GetVolumeId()---------------------------->  : ", req.GetVolumeId())
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	iscsiInfo, err := iscsi.getISCSIInfo(req)
	if err != nil {
		return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		iscsiInfo.isBlock = true
	}
	diskMounter := iscsi.getISCSIDiskMounter(iscsiInfo, req)
	if iscsiInfo.isBlock {
		diskMounter.bindMounter = mount.New("")
	}
	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		log.Errorf("Failed to mount volume with error %v", err)
		return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	// switch volCap.GetAccessType().(type) {
	// case *csi.VolumeCapability_Block:
	// 	ro := req.GetReadonly()
	// 	if ro {
	// 		return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, "Read only is not supported for Block Volume")
	// 	}
	// 	err = iscsi.BlockMount(*diskMounter)
	// 	if err != nil {
	// 		return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	// 	}
	// case *csi.VolumeCapability_Mount:
	// 	_, err = iscsi.AttachDisk(*diskMounter)
	// 	if err != nil {
	// 		return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	// 	}
	// }

	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) BlockMount(diskMounter iscsiDiskMounter) error {
	target := diskMounter.targetPath
	devicePath := diskMounter.sourcePath
	exists, err := diskMounter.mounter.ExistsPath(devicePath)
	if err != nil {
		log.Errorf("error finding device path %s", devicePath)
		return err
	}
	if !exists {
		log.Errorf("device path %s not found", devicePath)
		return err
	}
	// if !exists {
	// 	for _, portal := range diskMounter.Portals {
	// 		devicePath = strings.Join([]string{"/dev/disk/by-path/ip", portal, "iscsi", diskMounter.Iqn, "lun", fmt.Sprint(diskMounter.lun)}, "-")
	// 		stat, err := os.Lstat(devicePath)
	// 		if err != nil {
	// 			if os.IsNotExist(err) {
	// 				log.Infof("device path %q not found", devicePath)
	// 				return err
	// 			}
	// 			return err
	// 		}
	// 		if stat.Mode()&os.ModeSymlink != os.ModeSymlink {
	// 			log.Warningf("nvme file %q found, but was not a symlink", portal)
	// 			return err
	// 		}
	// 		resolved, err := filepath.EvalSymlinks(portal)
	// 		if err != nil {
	// 			log.Errorf("error reading target of symlink %q: %v", portal, err)
	// 			return err
	// 		}

	// 		if !strings.HasPrefix(resolved, "/dev") {
	// 			log.Errorf("resolved symlink for %q was unexpected: %q", portal, resolved)
	// 			return err
	// 		}
	// 		devicePath = resolved
	// 		break
	// 	}

	// }
	source := devicePath
	mountDir := filepath.Dir(target)

	if diskMounter.readOnly {
		return status.Error(codes.Internal, "Read only is not supported for Block Volume")
	}
	exists, err = diskMounter.mounter.ExistsPath(mountDir)
	if err != nil {
		return status.Errorf(codes.Internal, "Could not check if path exists %q: %v", mountDir, err)
	}
	if !exists {
		if err := diskMounter.mounter.MakeDir(mountDir); err != nil {
			return status.Errorf(codes.Internal, "Could not create dir %q: %v", mountDir, err)
		}
	}

	err = diskMounter.mounter.MakeFile(target)
	if err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return status.Errorf(codes.Internal, "Could not create file %q: %v", target, err)
	}

	options := []string{"bind"}
	if err := diskMounter.mounter.FormatAndMount(source, target, "", options); err != nil {
		return err
	}
	return nil
}

func (iscsi *iscsistorage) NodeUnpublishVolume(
	ctx context.Context,
	req *csi.NodeUnpublishVolumeRequest) (
	*csi.NodeUnpublishVolumeResponse, error) {
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req)
	targetPath := req.GetTargetPath()
	err := iscsi.DetachDisk(*diskUnmounter, targetPath)
	if err != nil {
		return &csi.NodeUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func createFile(filePath string) (bool, error) {
	st, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		file, err := os.Create(filePath)
		defer file.Close()
		if err != nil {
			log.Errorf("error while creating file %s", filePath)
			return false, err
		}
		log.Debugf("created file %s", filePath)
		return true, nil
	}
	if st.IsDir() {
		log.Error("provided path is directory not file")
		return false, fmt.Errorf("provided path is directory not file")
	}
	return false, nil
}
func createDirectory(dirPath string) (bool, error) {
	st, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		if err := os.Mkdir(dirPath, 0755); err != nil {
			log.Errorf("error while creating dir %s", dirPath)
			return false, err
		}
		log.Debugf("created directory %s", dirPath)
		return true, nil
	}
	if !st.IsDir() {
		return false, fmt.Errorf("not a directory")
	}
	return false, nil
}

func (iscsi *iscsistorage) AttachDisk(d iscsiDiskMounter) (string, error) {
	log.Info("In AttachDisk")
	log.Infof("mouting volume at %s", d.targetPath)
	log.WithFields(log.Fields{"iqn": d.iscsiDisk.Iqn, "lun": d.iscsiDisk.lun,
		"DoCHAPDiscovery": d.connector.DoCHAPDiscovery}).Info("Mounting Volume")

	devicePath, err := iscsi_lib.Connect(*d.connector)
	if err != nil {
		log.Errorf("Disk Connect failed with error %v", err)
		return "", err
	}
	if devicePath == "" {
		return "", fmt.Errorf("connect reported success, but no path returned")
	}

	// Mount device
	mntPath := d.targetPath
	notMnt, err := d.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("Heuristic determination of mount point failed:%v", err)
	}
	if !notMnt {
		log.Infof("iscsi: %s already mounted", mntPath)
		return "", nil
	}
	var options []string
	if d.isBlock {
		if d.readOnly {
			return "", status.Error(codes.Internal, "Read only is not supported for Block Volume")
		}

		err = d.bindMounter.MakeDir(filepath.Dir(d.targetPath))
		// err := os.MkdirAll(filepath.Dir(d.targetPath), 0777)
		// if err != nil {
		// 	log.Errorf("failed to create target directory %q: %v", filepath.Dir(d.targetPath), err)
		// 	return "", fmt.Errorf("failed to create target directory for raw block bind mount: %v", err)
		// }

		err = d.bindMounter.MakeFile(d.targetPath)
		// file, err := os.OpenFile(d.targetPath, os.O_CREATE, 0777)
		// if err != nil {
		// 	log.Errorf("failed to create target file %q: %v", d.targetPath, err)
		// 	return "", fmt.Errorf("failed to create target file for raw block bind mount: %v", err)
		// }
		// file.Close()

		symLink, err := filepath.EvalSymlinks(devicePath)
		if err != nil {
			log.Errorf("could not resolve symlink %q: %v", devicePath, err)
			return "", fmt.Errorf("could not resolve symlink %q: %v", devicePath, err)
		}

		if !strings.HasPrefix(symLink, "/dev") {
			log.Errorf("resolved symlink %q for %q was unexpected", symLink, devicePath)
			return "", fmt.Errorf("resolved symlink %q for %q was unexpected", symLink, devicePath)
		}
		options := []string{"bind"}
		options = append(options, "rw")
		if err := d.bindMounter.Mount(symLink, d.targetPath, "", options); err != nil {
			log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", symLink, d.fsType, d.targetPath, err)
			return "", err
		}
		err = d.bindMounter.MakeRShared(d.targetPath)
		if err != nil {
			log.Errorf("iscsi: MakeRShared failed for path %s and error %v", d.targetPath, err)
		}
		log.Info("Volume mounted successfully----------------------------------------> on ", d.targetPath)
		return symLink, err
	} else {
		log.Infof("Volume will be mounted at %s", d.targetPath)
		if err := os.MkdirAll(mntPath, 0750); err != nil {
			log.Errorf("iscsi: failed to mkdir %s, error", mntPath)
			return "", err
		}
		if d.readOnly {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		options = append(options, d.mountOptions...)
		log.Info("Trying to format and mount volume")
		err = d.mounter.FormatAndMount(devicePath, mntPath, d.fsType, options)
		if err != nil {
			log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, d.fsType, mntPath, err)
			return devicePath, err
		}
		log.Info("Volume mounted successfully")
		// Persist iscsi disk config to json file for DetachDisk path
		file := path.Join(mntPath, d.VolName+".json")
		d.connector.SessionSecrets = iscsi_lib.Secrets{}
		d.connector.DiscoverySecrets = iscsi_lib.Secrets{}
		err = iscsi_lib.PersistConnector(d.connector, file)
		if err != nil {
			log.Errorf("failed to persist connection info: %v", err)
			return "", fmt.Errorf("unable to create persistence file for connection")
		}
		log.Info("Volume mounted at %s", devicePath)
	}
	return devicePath, err
}

func (iscsi *iscsistorage) DetachDisk(c iscsiDiskUnmounter, targetPath string) error {
	// log.Println("DetachDisk------------------------->", targetPath)
	// _, cnt, err := mount.GetDeviceNameFromMount(c.mounter, targetPath)
	// log.Println("DetachDisk cnt------------------------->", cnt)
	// log.Println("DetachDisk err------------------------->", err)

	// if err != nil {
	// 	log.Errorf("iscsi detach disk: failed to get device from mnt: %s\nError: %v", targetPath, err)
	// 	return err
	// }
	// log.Println("DetachDisk pathExists------------------------->")
	// if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
	// 	return fmt.Errorf("Error checking if path exists: %v", pathErr)
	// } else if !pathExists {
	// 	log.Warningf("Warning: Unmount skipped because path does not exist: %v", targetPath)
	// 	return nil
	// }
	// log.Println("DetachDisk Unmount------------------------->")
	// if err = c.mounter.Unmount(targetPath); err != nil {
	// 	log.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
	// 	return err
	// }
	// log.Println("DetachDisk Unmount done------------------------->")
	// cnt--
	// if cnt != 0 {
	// 	return nil
	// }

	log.Debug("NodeUnpublishVolume")
	notMnt, err := c.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warnf("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
			return nil
		}
		return err
	}
	if notMnt {
		if err := os.Remove(targetPath); err != nil {
			log.Errorf("Remove target path error: %s", err.Error())
		}
		return nil
	}
	// load iscsi disk config from json file
	file := path.Join(targetPath, c.iscsiDisk.VolName+".json")
	if _, err := os.Stat(file); err == nil {
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
	}

	if err := c.mounter.Unmount(targetPath); err != nil {
		return err
	}
	// if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
	// 	return err
	// }

	if err := os.RemoveAll(targetPath); err != nil {
		log.Errorf("iscsi: failed to remove mount path Error: %v", err)
		return err
	}

	return nil
}

func (iscsi *iscsistorage) getISCSIInfo(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	tp := req.GetVolumeContext()["targetPortal"]
	iface := req.GetVolumeContext()["iscsiInterface"]
	iqn := req.GetVolumeContext()["iqn"]
	portals := req.GetVolumeContext()["portals"]
	lun := req.GetVolumeContext()["lun"]
	portalsList := strings.Split(portals, ",")
	secret := req.GetSecrets()
	sessionSecret, err := parseSessionSecret(secret)
	if err != nil {
		return nil, err
	}
	discoverySecret, err := parseDiscoverySecret(secret)
	// if err != nil {
	// 	return nil, err
	// }
	portal := iscsi.portalMounter(tp)
	var bkportal []string
	bkportal = append(bkportal, portal)
	for _, p := range portalsList {
		bkportal = append(bkportal, iscsi.portalMounter(p))
	}
	sourcePath := req.PublishContext["devicePath"]
	log.Println("req.PublishContext--------------------->", req.PublishContext)
	log.Println("getting devicePath---------------------------------->", sourcePath)
	initiatorName := iscsi.cs.getIscsiInitiatorName()

	doDiscovery := false
	chapDiscovery := false
	if req.GetVolumeContext()["chapAuthentication"] == "true" {
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
		sourcePath:      sourcePath,
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
		Interface:        iscsiInfo.Iface,
		CheckInterval:    2,
		RetryCount:       10,
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
	volproto := strings.Split(req.GetVolumeId(), "$$")
	return &iscsiDiskUnmounter{
		iscsiDisk: &iscsiDisk{
			VolName: volproto[0],
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

	//secret.SecretsType = "chap"
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
	sourcePath      string
	isBlock         bool
}

type iscsiDiskMounter struct {
	*iscsiDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	bindMounter  mount.Interface
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
	return &csi.NodeStageVolumeResponse{}, status.Error(codes.Unimplemented, time.Now().String()+"---  NodeStageVolume not implemented")
}
func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, status.Error(codes.Unimplemented, time.Now().String()+"---  NodeUnstageVolume not implemented")
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
	return &csi.NodeGetVolumeStatsResponse{}, status.Error(codes.Unimplemented, time.Now().String())

}

func (iscsi *iscsistorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return &csi.NodeExpandVolumeResponse{}, status.Error(codes.Unimplemented, time.Now().String())
}
