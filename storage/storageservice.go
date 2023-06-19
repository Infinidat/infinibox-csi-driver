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
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/api/clientgo"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"

	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

const (
	Name = "infinibox-csi-driver"
)

const (
	// for size conversion
	kib int64 = 1024
	mib int64 = kib * 1024
	gib int64 = mib * 1024
	// gib100 int64 = gib * 100
	tib int64 = gib * 1024
	// tib100 int64 = tib * 100
)

type Storageoperations interface {
	csi.ControllerServer
	csi.NodeServer
}

// Mutex protecting device rescan and delete operations
// var deviceMu sync.Mutex

type Commonservice struct {
	Api               api.Client
	storagePoolIdName map[int64]string
	driverversion     string
	AccessModesHelper helper.AccessModesHelper
}

type nfsstorage struct {
	uniqueID               int64
	storageClassParameters map[string]string
	pVName                 string
	capacity               int64
	fileSystemID           int64
	exportPath             string
	usePrivilegedPorts     bool
	snapdirVisible         bool
	exportID               int64
	exportBlock            string
	ipAddress              string
	cs                     Commonservice
	mounter                mount.Interface
	osHelper               helper.OsHelper
	storageHelper          StorageHelper
}

type treeqstorage struct {
	csi.ControllerServer
	csi.NodeServer
	treeqService TreeqInterface
	nfsstorage   nfsstorage
}

type fcstorage struct {
	cs            Commonservice
	configmap     map[string]string
	storageHelper StorageHelper
}

type iscsistorage struct {
	cs       Commonservice
	osHelper helper.OsHelper
}

// NewStorageController : To return specific implementation of storage
func NewStorageController(storageProtocol string, configparams ...map[string]string) (Storageoperations, error) {
	comnserv, err := BuildCommonService(configparams[0], configparams[1])
	if err == nil {
		storageProtocol = strings.ToLower(strings.TrimSpace(storageProtocol))
		switch storageProtocol {
		case common.PROTOCOL_FC:
			return &fcstorage{cs: comnserv, storageHelper: Service{}}, nil
		case common.PROTOCOL_ISCSI:
			return &iscsistorage{cs: comnserv, osHelper: helper.Service{}}, nil
		case common.PROTOCOL_NFS:
			return &nfsstorage{cs: comnserv, storageHelper: Service{}, osHelper: helper.Service{}}, nil
		case common.PROTOCOL_TREEQ:
			nfs := nfsstorage{storageClassParameters: make(map[string]string), cs: comnserv, storageHelper: Service{}, osHelper: helper.Service{}}
			service := &TreeqService{nfsstorage: nfs, cs: comnserv}
			return &treeqstorage{nfsstorage: nfs, treeqService: service}, nil
		default:
			return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
		}
	}
	return nil, err
}

// NewStorageNode : To return specific implementation of storage
func NewStorageNode(storageProtocol string, configparams ...map[string]string) (Storageoperations, error) {
	comnserv, err := BuildCommonService(configparams[0], configparams[1])
	if err == nil {
		storageProtocol = strings.ToLower(strings.TrimSpace(storageProtocol))
		switch storageProtocol {
		case common.PROTOCOL_FC:
			return &fcstorage{cs: comnserv, storageHelper: Service{}}, nil
		case common.PROTOCOL_ISCSI:
			return &iscsistorage{cs: comnserv, osHelper: helper.Service{}}, nil
		case common.PROTOCOL_NFS:
			return &nfsstorage{cs: comnserv, mounter: mount.NewWithoutSystemd(""), storageHelper: Service{}, osHelper: helper.Service{}}, nil
		case common.PROTOCOL_TREEQ:
			nfs := nfsstorage{storageClassParameters: make(map[string]string), cs: comnserv, mounter: mount.NewWithoutSystemd(""), storageHelper: Service{}, osHelper: helper.Service{}}
			service := &TreeqService{nfsstorage: nfs, cs: comnserv}
			return &treeqstorage{nfsstorage: nfs, treeqService: service}, nil
		default:
			return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
		}
	}
	return nil, err
}

func BuildCommonService(config map[string]string, secretMap map[string]string) (Commonservice, error) {
	commonserv := Commonservice{}
	if config != nil {
		if secretMap == nil || len(secretMap) < 3 {
			klog.Errorf("Api client cannot be initialized without proper secrets")
			return commonserv, errors.New("secrets are missing or not valid")
		}
		commonserv = Commonservice{
			Api: &api.ClientService{
				SecretsMap: secretMap,
			},
		}
		err := commonserv.verifyApiClient()
		if err != nil {
			klog.Errorf("API client not initialized, err: %v", err)
			return commonserv, err
		}
		commonserv.driverversion = config["driverversion"]
		commonserv.AccessModesHelper = helper.AccessMode{}
	}
	klog.V(2).Infof("buildCommonService commonservice configuration done. config %+v", config)
	return commonserv, nil
}

func (cs *Commonservice) verifyApiClient() error {
	klog.V(2).Infof("verifying api client")
	c, err := cs.Api.NewClient()
	if err != nil {
		klog.V(2).Infof("api client is not working.")
		return errors.New("failed to create rest client")
	}
	cs.Api = c
	klog.V(2).Infof("api client is verified.")
	return nil
}

// func (cs *commonservice) getIscsiInitiatorName() string {
// 	if ep, ok := csictx.LookupEnv(context.Background(), "ISCSI_INITIATOR_NAME"); ok {
// 		return ep
// 	}
// 	return ""
// }

func (cs *Commonservice) mapVolumeTohost(volumeID int, hostID int) (luninfo api.LunInfo, err error) {
	luninfo, err = cs.Api.MapVolumeToHost(hostID, volumeID, -1)
	if err != nil {
		if strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
			luninfo, err = cs.Api.GetLunByHostVolume(hostID, volumeID)
		}
		if err != nil {
			return luninfo, err
		}
	}
	return luninfo, nil
}

func (cs *Commonservice) unmapVolumeFromHost(hostID, volumeID int) (err error) {
	err = cs.Api.UnMapVolumeFromHost(hostID, volumeID)
	if err != nil {
		// Ignore the following errors
		successMsg := fmt.Sprintf("Success: No need to unmap volume with ID %d from host with ID %d", volumeID, hostID)
		if strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			klog.V(4).Infof("%s, host not found", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			klog.V(4).Infof("%s, lun not found", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			klog.V(4).Infof("%s, volume not found", successMsg)
			return nil
		}
		return err
	}
	return nil
}

func (cs *Commonservice) AddPortForHost(hostID int, portType, portName string) error {
	_, err := cs.Api.AddHostPort(portType, portName, hostID)
	if err != nil && !strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
		klog.Errorf("failed to add host port with error %v", err)
		return err
	}
	return nil
}

func (cs *Commonservice) AddChapSecurityForHost(hostID int, credentials map[string]string) error {
	_, err := cs.Api.AddHostSecurity(credentials, hostID)
	if err != nil {
		klog.Errorf("failed to add authentication for host %d with error %v", hostID, err)
		return err
	}
	return nil
}

func (cs *Commonservice) validateHost(hostName string) (*api.Host, error) {
	klog.V(2).Infof("Check if host available, create if not available")
	host, err := cs.Api.GetHostByName(hostName)
	if err != nil && !strings.Contains(err.Error(), "HOST_NOT_FOUND") {
		klog.Errorf("failed to get host with error %v", err)
		return nil, status.Errorf(codes.NotFound, "host not found: %s", hostName)
	}
	if host.ID == 0 {
		klog.V(2).Infof("Creating host with name: %s", hostName)
		host, err = cs.Api.CreateHost(hostName)
		if err != nil {
			klog.Errorf("failed to create host with error %v", err)
			return nil, status.Errorf(codes.Internal, "failed to create host: %s", hostName)
		}
	}
	return &host, nil
}

func (cs *Commonservice) getCSIResponse(vol *api.Volume, req *csi.CreateVolumeRequest) *csi.Volume {
	klog.V(2).Infof("getCSIResponse called with volume %+v", vol)
	storagePoolName := vol.PoolName
	if storagePoolName == "" {
		storagePoolName = cs.getStoragePoolNameFromID(vol.PoolId)
	}
	// Make the additional volume attributes
	attributes := map[string]string{
		"ID":              strconv.Itoa(vol.ID),
		"Name":            vol.Name,
		"StoragePoolID":   strconv.FormatInt(vol.PoolId, 10),
		"StoragePoolName": storagePoolName,
		"CreationTime":    time.Unix(int64(vol.CreatedAt), 0).String(),
		"targetWWNs":      req.GetParameters()["targetWWNs"],
	}
	vi := &csi.Volume{
		VolumeId:      strconv.Itoa(vol.ID),
		CapacityBytes: vol.Size,
		VolumeContext: attributes,
		ContentSource: req.GetVolumeContentSource(),
	}
	return vi
}

func (cs *Commonservice) getStoragePoolNameFromID(id int64) string {
	klog.V(2).Infof("getStoragePoolNameFromID called with storagepoolid %d", id)
	storagePoolName := cs.storagePoolIdName[id]
	if storagePoolName == "" {
		pool, err := cs.Api.FindStoragePool(id, "")
		if err == nil {
			storagePoolName = pool.Name
			cs.storagePoolIdName[id] = pool.Name
		} else {
			klog.Errorf("Could not found StoragePool: %d", id)
		}
	}
	return storagePoolName
}

func (cs *Commonservice) getNetworkSpaceIP(networkSpace string) (string, error) {
	nspace, err := cs.Api.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return "", err
	}
	if len(nspace.Portals) == 0 {
		return "", errors.New("ip address not found")
	}
	index := getRandomIndex(len(nspace.Portals))
	return nspace.Portals[index].IpAdress, nil
}

func getRandomIndex(max int) int {
	// rand.Seed(time.Now().UnixNano()) - not needed as of go1.20, automatically seeded by golang
	min := 0
	index := rand.Intn(max-min) + min
	return index
}

func (cs *Commonservice) GetCreatedBy() string {
	var createdBy string
	createdBy = "CSI/" + cs.driverversion
	k8version := getClusterVersion()
	if k8version != "" {
		createdBy = "CSI/" + k8version + "/" + cs.driverversion
	}
	return createdBy
}

func getClusterVersion() string {
	cl, err := clientgo.BuildClient()
	if err != nil {
		return ""
	}
	version, _ := cl.GetClusterVerion()
	return version
}

func detachMpathDevice(mpathDevice string, protocol string) error {
	var err error
	var devices []string
	dstPath := mpathDevice
	klog.V(4).Infof("detachMpathDevice() called with mpathDevice '%s' for protocol '%s'", mpathDevice, protocol)
	if dstPath != "" {
		if strings.HasPrefix(dstPath, "/host") {
			dstPath = strings.Replace(dstPath, "/host", "", 1)
		}

		if strings.HasPrefix(dstPath, "/dev/dm-") {
			devices, err = findSlaveDevicesOnMultipath(dstPath)
		} else {
			// Add single targetPath to devices
			devices = append(devices, dstPath)
		}

		if err != nil {
			return err
		}
		helper.PrettyKlogDebug("multipath devices", devices)

		lun, err := findLunOnDevice(devices[0])
		if err != nil {
			return err
		}

		mpath, err := findMpathFromDevice(mpathDevice)
		if err != nil {
			klog.Errorf("findMpathFromDevice for mpathDevice %s failed: %s", mpathDevice, err)
			return err
		}
		klog.V(4).Infof("mpath device is %s\n", mpath)

		// multipathFlush(mpath)

		// Warn if there are not exactly mpathDeviceCount devices
		if deviceCount := len(devices); deviceCount != mpathDeviceCount {
			klog.Warningf("Invalid mpath device count found while unstaging. Devices: %+v", devices)
		}

		hosts, err := findHosts(protocol)
		if err != nil {
			return err
		}

		_ = detachDiskByLun(hosts, lun)
	}
	klog.V(4).Infof("detachMpathDevice() completed with mpathDevice '%s' for protocol '%s'", mpathDevice, protocol)
	return nil
}

func removeFromScsiSubsystemByHostLun(host string, lun string) (err error) {
	targetsPath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:*:%s", host, lun)
	targets, err := filepath.Glob(targetsPath)
	if err != nil || len(targets) == 0 {
		klog.Warningf("No fc targets found at path %s: %+v", targetsPath, err)
		return nil
	}
	for _, targetString := range targets {
		target := strings.Split(targetString, ":")[2]
		_ = removeOneFromScsiSubsystemByHostLun(host, target, lun)
	}
	return nil
}

func removeOneFromScsiSubsystemByHostLun(host string, target string, lun string) (err error) {
	// fileName := "/sys/block/" + deviceName + "/device/delete"
	// klog.V(4).Infof("remove device from scsi-subsystem: path: %s", fileName)
	// data := []byte("1\n")
	// ioutil.WriteFile(fileName, data, 0666)
	// klog.V(4).Infof("Flush device '%s' output: %s", device, blockdevOut)

	defer func() {
		klog.V(4).Infof("removeFromScsiSubsystemByHostLun() with host %s, target %s and lun %s completed", host, target, lun)
	}()

	klog.V(4).Infof("removeFromScsiSubsystemByHostLun() called with host %s, target %s and lun %s", host, target, lun)

	deletePath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:%s:%s/device/delete", host, target, lun)
	statePath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:%s:%s/device/state", host, target, lun)
	var output string

	// Check device is in blocked state.
	var sleepCount time.Duration
	for i := 1; i <= 5; i++ {
		// Get state of device
		klog.V(4).Infof("Checking device state of %s", statePath)
		output, err = execScsi.Command("cat", statePath)
		if err != nil {
			klog.Errorf("Failed: Cannot check state of %s", statePath)
			return
		}
		deviceState := strings.TrimSpace(string(output))
		if deviceState == "blocked" {
			if i == 5 {
				msg := fmt.Sprintf("Device %s is blocked", statePath)
				klog.Errorf(msg)
				err = errors.New(msg)
				return
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			break
		}
	}

	// Echo 1 to delete device
	klog.V(4).Infof("Running 'echo 1 > %s'", deletePath)
	output, err = execScsi.Command("echo", fmt.Sprintf("1 > %s", deletePath))
	if err != nil {
		klog.Errorf("Failed to delete device '%s' with output '%s' and error '%v'", deletePath, output, err.Error())
		return
	}

	// Stat device
	if _, err := os.Stat(deletePath); err == nil {
		klog.Warningf("Device %s still exists", deletePath)
	} else if errors.Is(err, os.ErrNotExist) {
		klog.V(4).Infof("Device %s no longer exists", deletePath)
		return nil
	} else {
		klog.V(4).Infof("Device %s may or may not exist. See error: %s", deletePath, err)
	}

	return err
}

// detachDisk removes scsi device file such as /dev/sdX from the node.
func detachDiskByLun(hosts []string, lun string) error {
	defer func() {
		klog.V(4).Infof("detachDiskByLun() with hosts '%+v' and lun %s completed", hosts, lun)
		klog.Flush()
		// deviceMu.Unlock()
		// May happen if unlocking a mutex that was not locked
		klog.V(4).Infof("detachDiskByLun succeeded for lun '%s'", lun)
	}()

	klog.V(4).Infof("detachDiskByLun() called with hosts %+v and lun %s", hosts, lun)
	var err error

	for _, host := range hosts {
		err = removeFromScsiSubsystemByHostLun(host, lun)
	}
	return err
}

func waitForDeviceState(hostId string, lun string, state string) (err error) {
	targetsPath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:*:%s", hostId, lun)
	targets, err := filepath.Glob(targetsPath)
	if err != nil || len(targets) == 0 {
		klog.Warningf("No fc targets found at path %s: %+v", targetsPath, err)
		return nil
	}
	for _, targetString := range targets {
		target := strings.Split(targetString, ":")[2]
		_ = waitForOneDeviceState(hostId, target, lun, state)
	}
	return nil
}

func waitForOneDeviceState(hostId string, target string, lun string, state string) error {
	// Wait for device to be in state.
	var sleepCount time.Duration = 1
	hostPath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:%s:%s/device/state", hostId, target, lun)
	wwidPath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:%s:%s/device/wwid", hostId, target, lun)

	klog.V(4).Infof("Checking device state within %s", hostPath)
	for i := 1; i <= 5; i++ {
		// Get state of device
		hostOutput, err := execScsi.Command("cat", hostPath)
		if err != nil {
			klog.Warningf("Failed (%d): Cannot check state of device file %s: %s", i, hostPath, err)
		}
		deviceState := strings.TrimSpace(string(hostOutput))

		// Get wwid of device
		wwidOutput, err := execScsi.Command("cat", wwidPath)
		if err != nil {
			klog.Warningf("Failed (%d): Cannot get wwid of wwid file %s: %s", i, wwidPath, err)
		} else {
			wwid := strings.TrimSpace(string(wwidOutput))
			klog.V(4).Infof("Device %s has wwid '%s'", wwidPath, wwid)
		}

		if err != nil || deviceState != state {
			if i == 5 {
				msg := fmt.Sprintf("Device %s is not in state '%s'. Current state is '%s'", hostPath, state, deviceState)
				klog.Warning(msg)
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			klog.V(4).Infof("Device %s is in state '%s'", hostPath, state)
			break
		}
	}
	return nil
}

func waitForMultipath(hostId string, lun string) error {
	var sleepCount time.Duration = 1
	masterPath := fmt.Sprintf("/sys/class/scsi_disk/%s:0:*:%s/device/block/*/holders/*/slaves/*", hostId, lun)
	loopCount := 7
	for i := 1; i <= loopCount; i++ {
		devices, err := filepath.Glob(masterPath)
		if err != nil {
			klog.V(4).Infof("Failed to Glob devices using path '%s': %+v", masterPath, err)
		} else {
			klog.V(4).Infof("Glob devices '%s'", devices)
		}

		if err != nil || len(devices) < mpathDeviceCount {
			if i == loopCount {
				msg := fmt.Sprintf("Multipath device found only %d devices for host ID '%s' and lun '%s'", len(devices), hostId, lun)
				klog.Warning(msg)
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			break
		}
	}

	klog.V(4).Infof("Multipath device is online for host ID %s and lun '%s'", hostId, lun)
	return nil
}

// FindSlaveDevicesOnMultipath returns all slaves on the multipath device given the device path
func findSlaveDevicesOnMultipath(dm string) ([]string, error) {
	var devices []string
	// Split path /dev/dm-1 into "", "dev", "dm-1"
	parts := strings.Split(dm, "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "dev") {
		err := fmt.Errorf("findSlaveDevicesOnMultipath() for dm '%s' failed", dm)
		klog.Error(err.Error())
		return nil, err
	}
	disk := parts[2]
	slavesPath := path.Join("/sys/block/", disk, "/slaves/")

	files, err := os.ReadDir(slavesPath)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		devices = append(devices, path.Join("/dev/", f.Name()))
	}
	if len(devices) == 0 {
		err := fmt.Errorf("findSlaveDevicesOnMultipath() for dm %s found no devices", dm)
		klog.Error(err.Error())
		return nil, err
	}
	return devices, nil
}

func findHosts(protocol string) ([]string, error) {
	// TODO - Must use portals if supporting more than one target IQN.
	// Find hosts
	if protocol == common.PROTOCOL_ISCSI {
		hostIds, err := execScsi.Command("iscsiadm", fmt.Sprintf("-m session -P3 | awk '{ if (NF > 3 && $1 == \"Host\" && $2 == \"Number:\") printf(\"%%s \", $3) }'"))
		hosts := strings.Fields(hostIds)
		if err != nil {
			klog.Errorf("Finding hosts failed: %s", err)
			return hosts, err
		}
		if len(hosts) != mpathDeviceCount {
			klog.Warningf("The number of hosts is not %d. hosts: '%v'", mpathDeviceCount, hosts)
		}
		return hosts, nil
	} else if protocol == common.PROTOCOL_FC {
		pathLeader := "/sys/class/fc_host/host"
		hostsPath := fmt.Sprintf("%s*", pathLeader)
		foundHosts, err := filepath.Glob(hostsPath)
		if err != nil || len(foundHosts) == 0 {
			klog.Errorf("No fc hosts found at path %s", hostsPath)
		}
		hosts := []string{}
		for _, host := range foundHosts {
			fcHost := strings.Replace(host, pathLeader, "", -1)
			hosts = append(hosts, fcHost)
		}
		return hosts, nil
	}
	err := fmt.Errorf("unsupported protocol: %s", protocol)
	klog.Errorf(err.Error())
	return nil, err
}

// FindSlaveDevicesOnMultipath returns all slaves on the multipath device given the device path
func findLunOnDevice(devicePath string) (string, error) {
	var lun string
	// Split path /dev/sdaa into "", "dev", "sdaa"
	parts := strings.Split(devicePath, "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "dev") {
		return "", fmt.Errorf("invalid device name %s", devicePath)
	}
	device := parts[2]
	scsiDevicePath := fmt.Sprintf("/sys/class/block/%s/device/scsi_device", device)

	files, err := os.ReadDir(scsiDevicePath)
	if err != nil {
		return "", fmt.Errorf("cannot read scsi device path %s", scsiDevicePath)
	}

	hctl := files[0].Name()
	partsLun := strings.Split(hctl, ":")
	lun = partsLun[3]
	return lun, nil
}

/**
func (cs *commonservice) ExecuteWithTimeout(mSeconds int, command string, args []string) ([]byte, error) {
	klog.V(4).Infof("Executing command : {%v} with args : {%v}. and timeout : {%v} mseconds", command, args, mSeconds)

	// Create a new context and add a timeout to it
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(mSeconds)*time.Millisecond)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	// Create the command with our context
	cmd := exec.CommandContext(ctx, command, args...)

	// This time we can simply use Output() to get the result.
	out, err := cmd.Output()

	// We want to check the context error to see if the timeout was executed.
	// The error returned by cmd.Output() will be OS specific based on what
	// happens when a process is killed.
	if ctx.Err() == context.DeadlineExceeded {
		klog.V(4).Infof("Command %s timeout reached", command)
		return nil, ctx.Err()
	}

	// If there's no context error, we know the command completed (or errored).
	klog.V(4).Infof("Output from command: %s", string(out))
	if err != nil {
		klog.V(4).Infof("Non-zero exit code: %s", err)
	}

	klog.V(4).Infof("Finished executing command")
	return out, err
}
*/

func (cs *Commonservice) pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		klog.V(4).Infof("Path exists: %s", path)
		return true, nil
	} else if os.IsNotExist(err) {
		klog.V(4).Infof("Path not exists: %s", path)
		return false, nil
	} else if cs.isCorruptedMnt(err) {
		klog.V(4).Infof("Path is currupted: %s", path)
		return true, err
	} else {
		klog.V(4).Infof("unable to validate path: %s", path)
		return false, err
	}
}

func (cs *Commonservice) isCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pe := err.(type) {
	case nil:
		return false
	case *os.PathError:
		underlyingError = pe.Err
	case *os.LinkError:
		underlyingError = pe.Err
	case *os.SyscallError:
		underlyingError = pe.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO
}

func (st *fcstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (st *iscsistorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (st *nfsstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
