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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/amitosw15/infinibox-csi-driver/api"
	"github.com/amitosw15/infinibox-csi-driver/api/clientgo"
	"github.com/amitosw15/infinibox-csi-driver/common"
	"github.com/amitosw15/infinibox-csi-driver/helper"
	"github.com/amitosw15/infinibox-csi-driver/iboxapi"
	"net/url"
	"os/exec"

	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/go-logr/zerologr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

const (
	Name                  = "infinibox-csi-driver"
	RESTORE_TYPE_VOLUME   = "Volume"
	RESTORE_TYPE_SNAPSHOT = "Snapshot"
	TOBEDELETED           = "host.k8s.to_be_deleted"
)

// env vars that let users override various delay times
const (
	MULTIPATH_WAIT          = "MULTIPATH_WAIT"
	MULTIPATH_CLEANUP_DELAY = "MULTIPATH_CLEANUP_DELAY"
	FC_SEARCH_DISK_DELAY    = "FC_SEARCH_DISK_DELAY"
	RESIZE2FS_DELAY         = "RESIZE2FS_DELAY"
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
	ValidateStorageClass(params map[string]string) error
}

// Mutex protecting device rescan and delete operations
// var deviceMu sync.Mutex

type Commonservice struct {
	IboxApi           iboxapi.Client
	Api               api.Client
	storagePoolIdName map[int]string
	driverversion     string
	AccessModesHelper helper.AccessModesHelper
	VolProto          *api.VolumeProtocolConfig
}

type nfsstorage struct {
	uniqueID               int
	storageClassParameters map[string]string
	pVName                 string
	capacity               int64
	fileSystemID           int
	exportPath             string
	usePrivilegedPorts     bool
	snapdirVisible         bool
	exportID               int
	exportBlock            string
	ipAddress              string
	cs                     Commonservice
	mounter                mount.Interface
	osHelper               helper.OsHelper
	storageHelper          StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

type treeqstorage struct {
	csi.ControllerServer
	csi.NodeServer
	treeqService TreeqInterface
	nfsstorage   nfsstorage
}

type fcstorage struct {
	capacity      int64
	cs            Commonservice
	configmap     map[string]string
	storageHelper StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

type iscsistorage struct {
	capacity      int64
	cs            Commonservice
	osHelper      helper.OsHelper
	storageHelper StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

type nvmestorage struct {
	capacity      int64
	cs            Commonservice
	osHelper      helper.OsHelper
	storageHelper StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

type ShowMultipathOutput struct {
	MajorVersion int `json:"major_version"`
	MinorVersion int `json:"minor_version"`
	Map          struct {
		Name       string `json:"name"`
		UUID       string `json:"uuid"`
		Sysfs      string `json:"sysfs"`
		Failback   string `json:"failback"`
		Queueing   string `json:"queueing"`
		Paths      int    `json:"paths"`
		WriteProt  string `json:"write_prot"`
		DmSt       string `json:"dm_st"`
		Features   string `json:"features"`
		Hwhandler  string `json:"hwhandler"`
		Action     string `json:"action"`
		PathFaults int    `json:"path_faults"`
		Vend       string `json:"vend"`
		Prod       string `json:"prod"`
		Rev        string `json:"rev"`
		SwitchGrp  int    `json:"switch_grp"`
		MapLoads   int    `json:"map_loads"`
		TotalQTime int    `json:"total_q_time"`
		QTimeouts  int    `json:"q_timeouts"`
		PathGroups []struct {
			Selector   string `json:"selector"`
			Pri        int    `json:"pri"`
			DmSt       string `json:"dm_st"`
			MarginalSt string `json:"marginal_st"`
			Group      int    `json:"group"`
			Paths      []struct {
				Dev         string `json:"dev"`
				DevT        string `json:"dev_t"`
				DmSt        string `json:"dm_st"`
				DevSt       string `json:"dev_st"`
				ChkSt       string `json:"chk_st"`
				Checker     string `json:"checker"`
				Pri         int    `json:"pri"`
				HostWwnn    string `json:"host_wwnn"`
				TargetWwnn  string `json:"target_wwnn"`
				HostWwpn    string `json:"host_wwpn"`
				TargetWwpn  string `json:"target_wwpn"`
				HostAdapter string `json:"host_adapter"`
				MarginalSt  string `json:"marginal_st"`
			} `json:"paths"`
		} `json:"path_groups"`
	} `json:"map"`
}

// NewStorageController : To return specific implementation of storage
func NewStorageController(comnserv Commonservice, capacity int64, storageProtocol string, configparams ...map[string]string) (Storageoperations, error) {
	storageProtocol = strings.ToLower(strings.TrimSpace(storageProtocol))
	switch storageProtocol {
	case common.PROTOCOL_FC:
		return &fcstorage{capacity: capacity, cs: comnserv, storageHelper: StorageService{}}, nil
	case common.PROTOCOL_ISCSI:
		return &iscsistorage{capacity: capacity, cs: comnserv, osHelper: helper.Service{}}, nil
	case common.PROTOCOL_NVME:
		return &nvmestorage{capacity: capacity, cs: comnserv, osHelper: helper.Service{}}, nil
	case common.PROTOCOL_NFS:
		return &nfsstorage{capacity: capacity, cs: comnserv, storageHelper: StorageService{}, osHelper: helper.Service{}}, nil
	case common.PROTOCOL_TREEQ:
		nfs := nfsstorage{capacity: capacity, storageClassParameters: make(map[string]string), cs: comnserv, storageHelper: StorageService{}, osHelper: helper.Service{}}
		service := &TreeqService{nfsstorage: nfs, cs: comnserv}
		return &treeqstorage{nfsstorage: nfs, treeqService: service}, nil
	}
	return nil, errors.New("Error: Invalid storage protocol - " + storageProtocol)
}

// NewStorageNode : To return specific implementation of storage
func NewStorageNode(comnserv Commonservice, configparams ...map[string]string) (Storageoperations, error) {
	volProto := comnserv.VolProto

	storageProtocol := volProto.StorageType
	switch storageProtocol {
	case common.PROTOCOL_FC:
		return &fcstorage{cs: comnserv, storageHelper: StorageService{}}, nil
	case common.PROTOCOL_ISCSI:
		return &iscsistorage{cs: comnserv, osHelper: helper.Service{}, storageHelper: StorageService{}}, nil
	case common.PROTOCOL_NVME:
		return &nvmestorage{cs: comnserv, osHelper: helper.Service{}, storageHelper: StorageService{}}, nil
	case common.PROTOCOL_NFS:
		return &nfsstorage{cs: comnserv, mounter: mount.NewWithoutSystemd(""), storageHelper: StorageService{}, osHelper: helper.Service{}}, nil
	case common.PROTOCOL_TREEQ:
		//nfs := nfsstorage{storageClassParameters: make(map[string]string), cs: comnserv, mounter: mount.NewWithoutSystemd(""), storageHelper: Service{}, osHelper: helper.Service{}}
		nfs := nfsstorage{cs: comnserv, mounter: mount.NewWithoutSystemd(""), storageHelper: StorageService{}, osHelper: helper.Service{}}
		service := &TreeqService{nfsstorage: nfs, cs: comnserv}
		return &treeqstorage{nfsstorage: nfs, treeqService: service}, nil
	default:
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
}

func BuildCommonService(config map[string]string, secretMap map[string]string, volProto *api.VolumeProtocolConfig) (Commonservice, error) {
	commonserv := Commonservice{}
	if config != nil {
		if len(secretMap) < 3 {
			zlog.Error().Msgf("Api client cannot be initialized without proper secrets")
			return commonserv, errors.New("secrets are missing or not valid")
		}
		hostnameURL, err := url.Parse(secretMap["hostname"])

		if err != nil {
			zlog.Error().Msgf("Error parsing IBox hostname: %s", err.Error())
			return commonserv, errors.New("secret hostname is missing or not valid")
		}

		// check for scheme, add if missing.
		urlScheme := hostnameURL.Scheme

		var apiHost string
		if urlScheme == "" {
			zlog.Trace().Msgf("IBox Hostname is missing scheme, setting https as scheme")
			apiHost = "https://" + secretMap[common.CRED_HOSTNAME] + "/"
		} else {
			apiHost = hostnameURL.String()
		}

		// check for URI validity.
		hostnameURL, err = url.ParseRequestURI(apiHost)
		if err != nil {
			zlog.Error().Msgf("IBox hostname %s is invalid URI: %s", hostnameURL.String(), err.Error())
		} else {
			zlog.Trace().Msgf("IBox URL: %s", apiHost)
		}
		creds := iboxapi.Credentials{
			Username: secretMap[common.CRED_USERNAME],
			Password: secretMap[common.CRED_PASSWORD],
			Url:      apiHost,
		}
		var iboxApiLog = zerologr.New(&zlog)

		iboxapiClient := iboxapi.NewIboxClient(iboxApiLog, creds)
		commonserv = Commonservice{
			Api: &api.ClientService{
				SecretsMap: secretMap,
			},
			IboxApi:  iboxapiClient,
			VolProto: volProto,
		}
		err = commonserv.verifyApiClient()
		if err != nil {
			zlog.Error().Msgf("API client not initialized, err: %v", err)
			return commonserv, err
		}
		commonserv.driverversion = config["driverversion"]
		commonserv.AccessModesHelper = helper.AccessMode{}
	}
	zlog.Trace().Msgf("buildCommonService commonservice configuration done. config %+v", config)
	return commonserv, nil
}

func (cs *Commonservice) verifyApiClient() error {
	zlog.Trace().Msgf("verifying api client")
	c, err := cs.Api.NewClient()
	if err != nil {
		zlog.Error().Msgf("api client is not working.")
		return errors.New("failed to create rest client")
	}
	cs.Api = c
	zlog.Trace().Msgf("api client is verified.")
	return nil
}

func (cs *Commonservice) mapVolumeTohost(volumeID int, hostID int) (luninfo *iboxapi.LunInfo, err error) {
	luninfo, err = cs.IboxApi.MapVolumeToHost(hostID, volumeID, -1)
	if err != nil {
		if strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
			luninfo, err = cs.IboxApi.GetLunByHostVolume(hostID, volumeID)
		}
		if err != nil {
			return luninfo, err
		}
	}
	return luninfo, nil
}

func (cs *Commonservice) unmapVolumeFromHost(hostID, volumeID int) (err error) {
	_, err = cs.IboxApi.UnMapVolumeFromHost(hostID, volumeID)
	if err != nil {
		// Ignore the following errors
		successMsg := fmt.Sprintf("Success: No need to unmap volume with ID %d from host with ID %d", volumeID, hostID)
		if strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			zlog.Debug().Msgf("%s, host not found", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			zlog.Debug().Msgf("%s, lun not found", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			zlog.Debug().Msgf("%s, volume not found", successMsg)
			return nil
		}
		return err
	}
	return nil
}

func (cs *Commonservice) AddPortForHost(hostID int, portType, portName string) error {
	_, err := cs.IboxApi.AddHostPort(portType, portName, hostID)
	if err != nil && !strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
		zlog.Error().Msgf("failed to add host port with error %v", err)
		return err
	}
	return nil
}

func (cs *Commonservice) AddChapSecurityForHost(hostID int, credentials map[string]string) error {
	_, err := cs.IboxApi.AddHostSecurity(credentials, hostID)
	if err != nil {
		zlog.Error().Msgf("failed to add authentication for host %d with error %v", hostID, err)
		return err
	}
	return nil
}

func (cs *Commonservice) validateHost(hostName string) (*iboxapi.Host, error) {
	zlog.Debug().Msgf("Check if host available, create if not available")
	removeDomainName := os.Getenv("REMOVE_DOMAIN_NAME")
	if removeDomainName != "" && removeDomainName == "true" {
		shortName := strings.Split(hostName, ".")
		zlog.Debug().Msgf("REMOVE_DOMAIN_NAME set to true, %s resulting in %s", hostName, shortName[0])
		hostName = shortName[0]
	}
	host, err := cs.IboxApi.GetHostByName(hostName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("Creating host with name: %s", hostName)
			host, err = cs.IboxApi.CreateHost(hostName)
			if err != nil {
				zlog.Error().Msgf("failed to create host with error %v", err)
				return nil, status.Errorf(codes.Internal, "failed to create host: %s", hostName)
			}

			metadata := map[string]interface{}{
				common.CSI_CREATED_HOST: true,
			}
			_, err = cs.IboxApi.PutMetadata(host.ID, metadata)
			if err != nil {
				zlog.Error().Msgf("error creating host metadata : %s id %d error : %v", hostName, host.ID, err)
				return nil, err
			}
		} else {
			e := fmt.Errorf("validateHost - GetHostByName - hostname %s error %s", hostName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return host, nil
}

func (cs *Commonservice) getCSIResponse(vol *iboxapi.Volume, req *csi.CreateVolumeRequest) *csi.Volume {
	zlog.Debug().Msgf("getCSIResponse called with volume %+v", vol)
	storagePoolName := vol.PoolName
	if storagePoolName == "" {
		storagePoolName = cs.getStoragePoolNameFromID(vol.PoolId)
	}
	// Make the additional volume attributes
	attributes := map[string]string{
		"ID":              strconv.Itoa(vol.ID),
		"Name":            vol.Name,
		"StoragePoolID":   strconv.Itoa(vol.PoolId),
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

func (cs *Commonservice) getStoragePoolNameFromID(id int) string {
	zlog.Debug().Msgf("getStoragePoolNameFromID called with storagepoolid %d", id)
	storagePoolName := cs.storagePoolIdName[id]
	if storagePoolName == "" {
		pool, err := cs.IboxApi.GetPoolByID(id)
		if err == nil {
			storagePoolName = pool.Name
			cs.storagePoolIdName[id] = pool.Name
		} else {
			zlog.Error().Msgf("Could not found StoragePool: %d", id)
		}
	}
	return storagePoolName
}

func (cs *Commonservice) getNetworkSpaceIP(networkSpace string) (string, error) {
	nspace, err := cs.IboxApi.GetNetworkSpaceByName(networkSpace)
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
	var min int
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

// Flush a multipath device map for device.

func multipathFlush(mpath string) {
	zlog.Debug().Msgf("multipathFlush - Running multipath -f '%s'", mpath)

	isToLogOutput := true
	if out, _, err := execCommand.Command("multipath", fmt.Sprintf("-f %s", mpath), isToLogOutput); err != nil {
		zlog.Error().Msgf("multipathFlush - multipath -f '%s' failed - ignored: %s", mpath, err)
	} else {
		zlog.Debug().Msgf("multipathFlush - multipath -f '%s' succeeded: %s", mpath, out)
	}

	// _, _ = execScsi.Command("ls", "-l /host/dev/mapper/*; echo", isToLogOutput)
	// _, _ = execScsi.Command("ls", "/host/dev/sd*; echo", isToLogOutput)
}

// Given a device like '/dev/dm-0', find its matching multipath name such as 'mpathab'.
func findMpathFromDevice(device string) (mpath string, err error) {
	deviceName := strings.Replace(device, "/dev/", "", 1)
	wildcards := "\"%n_%d_\""
	command := fmt.Sprintf("multipathd show maps raw format %s | grep %s", wildcards, deviceName)
	out, _, err := execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("findMpathFromDevice (fc) - command: %s error: %s", command, err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("command [%s]", command)

	outParts := strings.Split(string(out), "_")
	if len(outParts) < 1 {
		e := fmt.Errorf("findMpathFromDevice - cannot correctly parse findMpathFromDevice: %s, out: %s", device, outParts)
		zlog.Error().Msg(e.Error())
		return mpath, e
	}
	if len(outParts) > 0 {
		mpath = outParts[0]
	}

	zlog.Debug().Msgf("findMpathFromDevice - device %s corresponds to multipath %s", device, mpath)
	return
}

func detachMpathDevice(mpathDevice string, protocol string) error {
	var err error
	var devices []string
	dstPath := mpathDevice
	var mpath string
	zlog.Debug().Msgf("detachMpathDevice() called with mpathDevice '%s' for protocol '%s'", mpathDevice, protocol)
	if dstPath != "" {
		if strings.HasPrefix(dstPath, "/host") {
			dstPath = strings.Replace(dstPath, "/host", "", 1)
		}

		if strings.Contains(dstPath, "mpath") {
			// getting mpath already passed
			mpath = mpathDevice
			devices, err = findDevicesForMpath(mpath)
			if err != nil {
				zlog.Error().Msgf("error looking for devices for multipath [%s]", mpath)
				return err
			}
		} else if strings.HasPrefix(dstPath, "/dev/dm-") {
			// older versions of the driver < 2.21.0 would pass a dm- device here instead of an mpath name
			devices, err = findSlaveDevicesOnMultipath(dstPath)
			if err != nil {
				zlog.Error().Msgf("error looking for slave devices for multipath [%s]", dstPath)
				return err
			}
			mpath, err = findMpathFromDevice(mpathDevice)
			if err != nil {
				zlog.Error().Msgf("findMpathFromDevice for mpathDevice %s failed: %s", mpathDevice, err)
				return err
			}
		} else {
			// Add single targetPath to devices
			devices = append(devices, dstPath)
		}

		helper.PrettyKlogDebug("multipath devices", devices)

		zlog.Debug().Msgf("mpath device is %s\n", mpath)

		// 1
		multipathFlush(mpath)

		const defaultSleepAfterFlush = 1
		sleepAfterFlushThisExecution := defaultSleepAfterFlush
		tmp := os.Getenv(MULTIPATH_CLEANUP_DELAY)
		if tmp != "" {
			userSpecifiedValue, err := strconv.Atoi(tmp)
			if err != nil {
				zlog.Error().Msgf("conversion of %s env var failed, using default value of %d instead", MULTIPATH_CLEANUP_DELAY, defaultSleepAfterFlush)
			} else {
				sleepAfterFlushThisExecution = userSpecifiedValue
				zlog.Warn().Msgf("using non-default value for %s env var, user has specified %d, default is %d", MULTIPATH_CLEANUP_DELAY, sleepAfterFlushThisExecution, defaultSleepAfterFlush)
			}
		}
		zlog.Debug().Msgf("sleeping in between flush of device and detach of scsi disks for %d seconds", sleepAfterFlushThisExecution)
		time.Sleep(time.Second * time.Duration(sleepAfterFlushThisExecution))
		zlog.Debug().Msg("after sleep")

		// Warn if there are not exactly mpathDeviceCount devices
		if deviceCount := len(devices); deviceCount != mpathDeviceCount {
			zlog.Warn().Msgf("Invalid mpath device count found while unstaging. Devices: %+v", devices)
		}

		// 2
		for i := range devices {
			err = detachDiskByDeviceName(devices[i])
			if err != nil {
				zlog.Error().Msgf("error : %s", err)
			}
		}

		// 3
		err = removeMultipathDevices(devices)
		if err != nil {
			zlog.Debug().Msgf("error but continuing: %s", err.Error())
		}

		// 4
		err = removeWWIDEntry(mpath)
		if err != nil {
			zlog.Debug().Msgf("error but continuing: %s", err.Error())
		}

	}
	zlog.Debug().Msgf("detachMpathDevice() completed with mpathDevice '%s' for protocol '%s'", mpathDevice, protocol)
	return nil
}

func removeOneFromScsiSubsystemByHostLun(host string, channel string, target string, lun string) (err error) {
	// fileName := "/sys/block/" + deviceName + "/device/delete"
	// zlog.Debug().Msgf("remove device from scsi-subsystem: path: %s", fileName)
	// data := []byte("1\n")
	// ioutil.WriteFile(fileName, data, 0666)
	// zlog.Debug().Msgf("Flush device '%s' output: %s", device, blockdevOut)

	defer func() {
		zlog.Debug().Msgf("removeFromScsiSubsystemByHostLun() with host %s, channel %s, target %s and lun %s completed", host, channel, target, lun)
	}()

	zlog.Debug().Msgf("removeFromScsiSubsystemByHostLun() called with host %s, channel %s target %s and lun %s", host, channel, target, lun)

	deletePath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/delete", host, channel, target, lun)
	statePath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/state", host, channel, target, lun)
	var output string

	// Check device is in blocked state.
	var sleepCount time.Duration
	for i := 1; i <= 5; i++ {
		// Get state of device
		zlog.Debug().Msgf("Checking device state of %s", statePath)
		output, _, err = execCommand.Command("cat", statePath)
		if err != nil {
			zlog.Error().Msgf("Failed: Cannot check state of %s", statePath)
			return
		}
		deviceState := strings.TrimSpace(string(output))
		if deviceState == "blocked" {
			if i == 5 {
				msg := fmt.Sprintf("Device %s is blocked", statePath)
				zlog.Error().Msg(msg)
				err = errors.New(msg)
				return
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			break
		}
	}

	// Echo 1 to delete device
	output, _, err = execCommand.Command("echo", fmt.Sprintf("1 > %s", deletePath))
	if err != nil {
		zlog.Error().Msgf("Failed to delete device '%s' with output '%s' and error '%v'", deletePath, output, err.Error())
		return
	}

	// Stat device
	if _, err := os.Stat(deletePath); err == nil {
		zlog.Warn().Msgf("Device %s still exists", deletePath)
	} else if errors.Is(err, os.ErrNotExist) {
		zlog.Debug().Msgf("Device %s no longer exists", deletePath)
		return nil
	} else {
		zlog.Debug().Msgf("Device %s may or may not exist. See error: %s", deletePath, err)
	}

	return err
}

func detachDiskByDeviceName(deviceName string) error {

	// we get in a device name like /dev/sda
	zlog.Debug().Msgf("detachDiskByDeviceName(%s) called", deviceName)
	deviceNameParts := strings.Split(deviceName, "/")
	if len(deviceNameParts) != 3 {
		return fmt.Errorf("device name %s did not parse to 3 parts as normal", deviceName)
	}
	zlog.Trace().Msgf("deviceNameParts length = %d, parts are [%v] one=[%s]", len(deviceNameParts), deviceNameParts, deviceNameParts[2])

	blockPath := fmt.Sprintf("/sys/block/%s/device", deviceNameParts[2])
	zlog.Debug().Msgf("blockpath [%s]", blockPath)
	hctlPath, err := filepath.EvalSymlinks(blockPath)
	if err != nil {
		return err
	}

	// here we are expecting hctlPath to be similar to:
	// /sys/devices/pci0000:00/0000:00:15.0/0000:03:00.0/host32/rport-32:0-7/target32:0:9/32:0:9:1
	// we want the last part which is the H:C:T:L

	hctlPathParts := strings.Split(hctlPath, "/")

	hctl := hctlPathParts[len(hctlPathParts)-1]
	zlog.Trace().Msgf("hctl path [%s] - parsed as [%s]", hctlPath, hctl)

	hctlParts := strings.Split(hctl, ":")

	host := hctlParts[0]
	channel := hctlParts[1]
	target := hctlParts[2]
	lun := hctlParts[3]
	zlog.Debug().Msgf("hctl path [%s] host [%s] channel [%s] target [%s] lun [%s]", hctlPath, host, channel, target, lun)
	err = removeOneFromScsiSubsystemByHostLun(host, channel, target, lun)
	if err != nil {
		return err
	}

	return nil
}

func waitForDeviceState(hostId string, lun string, state string, diskid string) (wwid string, err error) {
	zlog.Debug().Msgf("waitForDeviceState hostid %s lun %s state %s diskid %s", hostId, lun, state, diskid)
	targetsPath := fmt.Sprintf("/sys/class/scsi_disk/%s:*:*:%s", hostId, lun)
	targets, err := filepath.Glob(targetsPath)
	if err != nil || len(targets) == 0 {
		zlog.Warn().Msgf("No fc targets found at path %s: %+v", targetsPath, err)
		return "", nil
	}

	allWWIDs := make([]string, 0)

	for _, targetString := range targets {
		target := strings.Split(targetString, ":")[2]
		channel := strings.Split(targetString, ":")[1]
		wwid, _ = waitForOneDeviceState(hostId, channel, target, lun, state)
		allWWIDs = append(allWWIDs, wwid)
	}

	if len(allWWIDs) == 1 {
		zlog.Debug().Msgf("only 1 wwid found [%s]", wwid)
		return wwid, nil
	}

	if len(allWWIDs) == 0 {
		return "", fmt.Errorf("no wwid found")
	}

	// this logic uses the disk id to find the correct wwid when there
	// are multiple wwids for different targets.
	// in this case, the disk id looks like:
	// 5742b0f0000bbd11
	// note: that the disk ID is really the FC port Target WWPN on an ibox
	// and the wwids look like naa.6742b0f000000bbd00000000002b3e13
	// we parse out enough unique FC port characters (the key) from the diskid to perform a fuzzy search with
	// on the wwid, in this example 'bdd' is the key we use to search for the correct wwid
	zlog.Debug().Msgf("picking the wwid from multiple wwid - diskid %s allWWIDs are [%v]", diskid, allWWIDs)
	n := 5
	lastChars := diskid[len(diskid)-n:]
	key := lastChars[:3]
	for i := range allWWIDs {
		if strings.Contains(allWWIDs[i], key) {
			zlog.Debug().Msgf("matched wwid using diskid %s key %s, wwid %s", diskid, key, allWWIDs[i])
			return allWWIDs[i], nil
		}
	}

	return "", fmt.Errorf("could not determine the wwid")
}

func waitForOneDeviceState(hostId string, channel string, target string, lun string, state string) (string, error) {
	zlog.Debug().Msgf("waitForOneDeviceState hostid %s target %s lun %s state %s", hostId, target, lun, state)
	// Wait for device to be in state.
	var sleepCount time.Duration = 1
	hostPath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/state", hostId, channel, target, lun)
	wwidPath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/wwid", hostId, channel, target, lun)

	var wwid string
	zlog.Debug().Msgf("Checking device state within %s", hostPath)
	for i := 1; i <= 5; i++ {
		// Get state of device
		hostOutput, _, err := execCommand.Command("cat", hostPath)
		if err != nil {
			zlog.Warn().Msgf("Failed (%d): Cannot check state of device file %s: %s", i, hostPath, err)
		}
		deviceState := strings.TrimSpace(string(hostOutput))

		// Get wwid of device
		wwidOutput, _, err := execCommand.Command("cat", wwidPath)
		if err != nil {
			zlog.Warn().Msgf("Failed (%d): Cannot get wwid of wwid file %s: %s", i, wwidPath, err)
		} else {
			wwid = strings.TrimSpace(string(wwidOutput))
			zlog.Debug().Msgf("Device %s has wwid '%s'", wwidPath, wwid)
		}

		if err != nil || deviceState != state {
			if i == 5 {
				msg := fmt.Sprintf("Device %s is not in state '%s'. Current state is '%s'", hostPath, state, deviceState)
				zlog.Warn().Msg(msg)
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			zlog.Debug().Msgf("Device %s is in state '%s'", hostPath, state)
			break
		}
	}
	return wwid, nil
}

func waitForMultipath(hostId string, lun string) error {
	defer helper.TimeTrack(zlog, time.Now())
	const defaultMultipathWait = 250
	var sleepCount time.Duration
	sleepCount = time.Duration(defaultMultipathWait)
	tmp := os.Getenv(MULTIPATH_WAIT)
	if tmp != "" {
		userSpecifiedValue, err := strconv.Atoi(tmp)
		if err != nil {
			zlog.Error().Msgf("error converting user specified env var %s, using default value of %d instead", MULTIPATH_WAIT, defaultMultipathWait)
		} else {
			zlog.Warn().Msgf("using non-default value for %s env var, user has specified %d, default is %d", MULTIPATH_WAIT, userSpecifiedValue, defaultMultipathWait)
			sleepCount = time.Duration(userSpecifiedValue)
		}
	}
	masterPath := fmt.Sprintf("/sys/class/scsi_disk/%s:*:*:%s/device/block/*/holders/*/slaves/*", hostId, lun)
	loopCount := 40
	for i := 1; i <= loopCount; i++ {
		zlog.Trace().Msgf("looping in waitForMultipath host %s lun %s", hostId, lun)
		devices, err := filepath.Glob(masterPath)
		if err != nil {
			zlog.Debug().Msgf("failed to glob devices using path '%s': %+v", masterPath, err)
		} else {
			zlog.Trace().Msgf("glob devices '%s'", devices)
		}

		if err != nil || len(devices) < mpathDeviceCount {
			if i == loopCount {
				msg := fmt.Sprintf("Multipath device found only %d devices for host ID '%s' and lun '%s'", len(devices), hostId, lun)
				zlog.Warn().Msg(msg)
			}
			time.Sleep(sleepCount * time.Millisecond)
		} else {
			break
		}
	}

	zlog.Debug().Msgf("multipath device is online for host ID %s and lun '%s'", hostId, lun)
	return nil
}

// FindSlaveDevicesOnMultipath returns all slaves on the multipath device given the device path
func findSlaveDevicesOnMultipath(dm string) ([]string, error) {
	var devices []string
	// Split path /dev/dm-1 into "", "dev", "dm-1"
	parts := strings.Split(dm, "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "dev") {
		err := fmt.Errorf("findSlaveDevicesOnMultipath() for dm '%s' failed", dm)
		zlog.Error().Msg(err.Error())
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
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	return devices, nil
}

func findHosts(protocol string) ([]string, error) {
	// TODO - Must use portals if supporting more than one target IQN.
	// Find hosts
	switch protocol {
	case common.PROTOCOL_ISCSI:
		hostIds, _, err := execCommand.Command("iscsiadm", fmt.Sprintf("-m session -P3 | awk '{ if (NF > 3 && $1 == \"Host\" && $2 == \"Number:\") printf(\"%%s \", $3) }'"))
		hosts := strings.Fields(hostIds)
		if err != nil {
			zlog.Error().Msgf("Finding hosts failed: %s", err)
			return hosts, err
		}
		if len(hosts) != mpathDeviceCount {
			zlog.Warn().Msgf("The number of hosts is not %d. hosts: '%v'", mpathDeviceCount, hosts)
		}
		return hosts, nil
	case common.PROTOCOL_FC:
		pathLeader := "/sys/class/fc_host/host"
		hostsPath := fmt.Sprintf("%s*", pathLeader)
		foundHosts, err := filepath.Glob(hostsPath)
		if err != nil || len(foundHosts) == 0 {
			zlog.Error().Msgf("No fc hosts found at path %s", hostsPath)
		}
		hosts := []string{}
		for _, host := range foundHosts {
			fcHost := strings.ReplaceAll(host, pathLeader, "")
			hosts = append(hosts, fcHost)
		}
		return hosts, nil
	}

	err := fmt.Errorf("unsupported protocol: %s", protocol)
	zlog.Error().Msg(err.Error())
	return nil, err
}

func (cs *Commonservice) pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		zlog.Debug().Msgf("Path exists: %s", path)
		return true, nil
	} else if os.IsNotExist(err) {
		zlog.Debug().Msgf("Path not exists: %s", path)
		return false, nil
	} else if cs.isCorruptedMnt(err) {
		zlog.Debug().Msgf("Path is currupted: %s", path)
		return true, err
	} else {
		zlog.Debug().Msgf("unable to validate path: %s", path)
		return false, err
	}
}

func (cs *Commonservice) isCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pe := err.(type) {
	case *os.PathError:
		underlyingError = pe.Err
	case *os.LinkError:
		underlyingError = pe.Err
	case *os.SyscallError:
		underlyingError = pe.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO
}

func removeMultipathDevices(devices []string) error {
	zlog.Debug().Msgf("removeMultipathDevices() called with hosts %+v", devices)

	for _, device := range devices {
		command := fmt.Sprintf("multipathd del path %s", device)
		pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)

		//out, err := exec.Command("bash", "-c", pipefailCmd).CombinedOutput()

		// we only care about the stdout, you can get stderr output from multipath.conf (invalid and deprecated lines)
		out, err := exec.Command("bash", "-c", pipefailCmd).Output()
		if err != nil {
			zlog.Error().Msgf("%s command failed %s", command, err.Error())
		} else {
			zlog.Debug().Msgf("%s command succeeded %s", command, out)
		}
	}
	return nil
}

// removeWWIDEntry causes WWID entries to be removed/cleaned up in /etc/multipath/wwids
// this is accomplished by runnning 'multipath -w %s' (or WWID)'", mpath
func removeWWIDEntry(mpath string) error {
	command := fmt.Sprintf("multipath -w  %s", mpath)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	zlog.Debug().Msgf("command [%s]", command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		zlog.Error().Msgf("%s command failed %s", command, err.Error())
	} else {
		zlog.Debug().Msgf("%s command succeeded: %s", command, out)
	}
	return nil
}

func findDevicesForMpath(mpath string) (devices []string, err error) {
	command := fmt.Sprintf("multipathd show multipath %s json", mpath)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	zlog.Debug().Msgf("command [%s]", command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		e := fmt.Errorf("findDevicesForMpath - cannot findDevicesForMpath: %s, Error: %s", mpath, err)
		zlog.Error().Msg(e.Error())
		return devices, e
	}
	var mpathOutput ShowMultipathOutput
	err = json.Unmarshal(out, &mpathOutput)
	if err != nil {
		e := fmt.Errorf("findDevicesForMpath - error unmarshalling output: %s, error: %s", string(out), err)
		zlog.Error().Msg(e.Error())
		return devices, e
	}

	pathGroups := mpathOutput.Map.PathGroups
	for i := 0; i < len(pathGroups); i++ {
		paths := pathGroups[i]
		for j := 0; j < len(paths.Paths); j++ {
			devices = append(devices, "/dev/"+paths.Paths[j].Dev)
		}
	}

	zlog.Debug().Msgf("findDevicesForMpath - devices %v for multipath %s", devices, mpath)
	return devices, nil
}
