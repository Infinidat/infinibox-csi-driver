/*Copyright 2020 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"infinibox-csi-driver/api/clientgo"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csictx "github.com/rexray/gocsi/context"
	log "github.com/sirupsen/logrus"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	Name                   = "infinibox-csi-driver"
	bytesInKiB             = 1024
	KeyThickProvisioning   = "thickprovisioning"
	thinProvisioned        = "Thin"
	thickProvisioned       = "Thick"
	KeyVolumeProvisionType = "provision_type"
)

type Storageoperations interface {
	csi.ControllerServer
	csi.NodeServer
}

type fcstorage struct {
	cs commonservice
}
type iscsistorage struct {
	cs commonservice
}
type treeqstorage struct {
	csi.ControllerServer
	csi.NodeServer
	filesysService FileSystemInterface
	osHelper       helper.OsHelper
	mounter        mount.Interface
}
type nfsstorage struct {
	uniqueID  int64
	configmap map[string]string
	pVName    string
	capacity  int64

	////
	fileSystemID int64
	exportpath   string
	exportID     int64
	exportBlock  string
	ipAddress    string
	cs           commonservice
	mounter      mount.Interface
	osHelper     helper.OsHelper
}

type commonservice struct {
	api               api.Client
	storagePoolIdName map[int64]string
	driverversion     string
}

//NewStorageController : To return specific implementation of storage
func NewStorageController(storageProtocol string, configparams ...map[string]string) (Storageoperations, error) {
	comnserv, err := buildCommonService(configparams[0], configparams[1])
	if err == nil {
		storageProtocol = strings.TrimSpace(storageProtocol)
		if storageProtocol == "fc" {
			return &fcstorage{cs: comnserv}, nil
		} else if storageProtocol == "iscsi" {
			return &iscsistorage{cs: comnserv}, nil
		} else if storageProtocol == "nfs" {
			return &nfsstorage{cs: comnserv, mounter: mount.New(""), osHelper: helper.Service{}}, nil
		} else if storageProtocol == "nfs_treeq" {
			return &treeqstorage{filesysService: getFilesystemService(storageProtocol, comnserv), osHelper: helper.Service{}}, nil
		}
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
	return nil, err
}

//NewStorageNode : To return specific implementation of storage
func NewStorageNode(storageProtocol string, configparams ...map[string]string) (Storageoperations, error) {
	comnserv, err := buildCommonService(configparams[0], configparams[1])
	if err == nil {
		storageProtocol = strings.TrimSpace(storageProtocol)
		if storageProtocol == "fc" {
			return &fcstorage{cs: comnserv}, nil
		} else if storageProtocol == "iscsi" {
			return &iscsistorage{cs: comnserv}, nil
		} else if storageProtocol == "nfs" {
			return &nfsstorage{cs: comnserv, mounter: mount.New(""), osHelper: helper.Service{}}, nil
		} else if storageProtocol == "nfs_treeq" {
			return &treeqstorage{filesysService: getFilesystemService(storageProtocol, comnserv), mounter: mount.New(""), osHelper: helper.Service{}}, nil
		}
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
	return nil, err
}

func buildCommonService(config map[string]string, secretMap map[string]string) (commonservice, error) {
	commonserv := commonservice{}
	if config != nil {
		if secretMap == nil || len(secretMap) < 3 {
			log.Error("Api client cannot be initialized without proper secrets")
			return commonserv, errors.New("secrets are missing or not valid")
		}
		commonserv = commonservice{
			api: &api.ClientService{
				SecretsMap: secretMap,
			},
		}
		err := commonserv.verifyApiClient()
		if err != nil {
			log.Error("API client not initialized.", err)
			return commonserv, err
		}
		commonserv.driverversion = config["driverversion"]
	}
	log.Infoln("buildCommonService commonservice configuration done.")
	return commonserv, nil
}

func (cs *commonservice) verifyApiClient() error {
	log.Info("verifying api client")
	c, err := cs.api.NewClient()
	if err != nil {
		log.Info("api client is not working.")
		return errors.New("failed to create rest client")
	}
	cs.api = c
	log.Info("api client is verified.")
	return nil
}
func (cs *commonservice) getIscsiInitiatorName() string {
	if ep, ok := csictx.LookupEnv(context.Background(), "ISCSI_INITIATOR_NAME"); ok {
		return ep
	}
	return ""
}
func (cs *commonservice) getVolumeByID(id int) (*api.Volume, error) {

	// The `GetVolume` API returns a slice of volumes, but when only passing
	// in a volume ID, the response will be just the one volume
	vols, err := cs.api.GetVolume(id)
	if err != nil {
		return nil, err
	}
	return vols, nil
}

func (cs *commonservice) mapVolumeTohost(volumeID int, hostID int) (luninfo api.LunInfo, err error) {
	luninfo, err = cs.api.MapVolumeToHost(hostID, volumeID, -1)
	if err != nil {
		if strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
			luninfo, err = cs.api.GetLunByHostVolume(hostID, volumeID)
		}
		if err != nil {
			return luninfo, err
		}
	}
	return luninfo, nil
}

func (cs *commonservice) unmapVolumeFromHost(hostID, volumeID int) (err error) {
	err = cs.api.UnMapVolumeFromHost(hostID, volumeID)
	if err != nil {
		// ignoring following error
		if strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			log.Debugf("cannot unmap volume from host with id %d, host not found", hostID)
			return nil
		} else if strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			log.Debugf("cannot unmap volume with id %d from host id %d , lun not found", volumeID, hostID)
			return nil
		} else if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			log.Debugf("volume with ID %d is already deleted , volume not found", volumeID)
			return nil
		}
		return err
	}
	return nil
}

func (cs *commonservice) AddPortForHost(hostID int, portType, portName string) error {
	_, err := cs.api.AddHostPort(portType, portName, hostID)
	if err != nil && !strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
		log.Errorf("failed to add host port with error %v", err)
		return err
	}
	return nil
}

func (cs *commonservice) AddChapSecurityForHost(hostID int, credentials map[string]string) error {
	_, err := cs.api.AddHostSecurity(credentials, hostID)
	if err != nil {
		log.Errorf("failed to add authentication for host %d with error %v", hostID, err)
		return err
	}
	return nil
}

func (cs *commonservice) validateHost(hostName string) (*api.Host, error) {
	log.Info("Check if host available, create if not available")
	host, err := cs.api.GetHostByName(hostName)
	if err != nil && !strings.Contains(err.Error(), "HOST_NOT_FOUND") {
		log.Errorf("failed to get host with error %v", err)
		return nil, err
	}
	if host.ID == 0 {
		log.Info("Creating host with name ", hostName)
		host, err = cs.api.CreateHost(hostName)
		if err != nil {
			log.Errorf("failed to create host with error %v", err)
			return nil, err
		}
	}
	return &host, nil
}

func (cs *commonservice) deleteVolume(volumeID int) (err error) {
	err = cs.api.DeleteVolume(volumeID)
	if err != nil {
		return err
	}
	return nil
}

func (cs *commonservice) getCSIResponse(vol *api.Volume, req *csi.CreateVolumeRequest) *csi.Volume {
	log.Infof("getCSIResponse called with vol %v", vol)
	storagePoolName := vol.PoolName
	log.Infof("getCSIResponse storagePoolName is %s", vol.PoolName)
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

func (cs *commonservice) getStoragePoolNameFromID(id int64) string {
	log.Infof("getStoragePoolNameFromID called with storagepoolid %d", id)
	storagePoolName := cs.storagePoolIdName[id]
	if storagePoolName == "" {
		pool, err := cs.api.FindStoragePool(id, "")
		if err == nil {
			storagePoolName = pool.Name
			cs.storagePoolIdName[id] = pool.Name
		} else {
			log.Errorf("Could not found StoragePool: %d", id)
		}
	}
	return storagePoolName
}

func (cs *commonservice) getNetworkSpaceIP(networkSpace string) (string, error) {
	nspace, err := cs.api.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return "", err
	}
	if len(nspace.Portals) == 0 {
		return "", errors.New("Ip address not found")
	}
	index := getRandomIndex(len(nspace.Portals))
	return nspace.Portals[index].IpAdress, nil
}

func getRandomIndex(max int) int {
	rand.Seed(time.Now().UnixNano())
	min := 0
	index := rand.Intn(max-min) + min
	return index
}

func (cs *commonservice) GetCreatedBy() string {
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

func GetUnixPermission(unixPermission, defaultPermission string) (os.FileMode, error) {
	var mode os.FileMode
	if unixPermission == "" {
		unixPermission = defaultPermission
	}
	i, err := strconv.ParseUint(unixPermission, 8, 32)
	if err != nil {
		log.Errorf("fail to cast unixPermission %v", err)
		return mode, err
	}
	mode = os.FileMode(i)
	return mode, nil
}

// detachFCDisk removes scsi device file such as /dev/sdX from the node.
func detachDisk(devicePath string) error {
	// Remove scsi device from the node.
	if !strings.HasPrefix(devicePath, "/dev/") {
		return fmt.Errorf("detach disk: invalid device name: %s", devicePath)
	}
	arr := strings.Split(devicePath, "/")
	dev := arr[len(arr)-1]
	removeFromScsiSubsystem(dev)
	return nil
}

// Removes a scsi device based upon /dev/sdX name
func removeFromScsiSubsystem(deviceName string) {
	fileName := "/sys/block/" + deviceName + "/device/delete"
	log.Infof("remove device from scsi-subsystem: path: %s", fileName)
	data := []byte("1")
	ioutil.WriteFile(fileName, data, 0666)
}

//FindSlaveDevicesOnMultipath returns all slaves on the multipath device given the device path
func findSlaveDevicesOnMultipath(dm string) []string {
	var devices []string
	// Split path /dev/dm-1 into "", "dev", "dm-1"
	parts := strings.Split(dm, "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "dev") {
		return devices
	}
	disk := parts[2]
	slavesPath := path.Join("/sys/block/", disk, "/slaves/")
	if files, err := ioutil.ReadDir(slavesPath); err == nil {
		for _, f := range files {
			devices = append(devices, path.Join("/dev/", f.Name()))
		}
	}
	return devices
}

func (cs *commonservice) ExecuteWithTimeout(mSeconds int, command string, args []string) ([]byte, error) {
	log.Debugf("Executing command : {%v} with args : {%v}. and timeout : {%v} mseconds", command, args, mSeconds)

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
		log.Debugf("Command %s timeout reached", command)
		return nil, ctx.Err()
	}

	// If there's no context error, we know the command completed (or errored).
	log.Debugf("Output from command: %s", string(out))
	if err != nil {
		log.Debugf("Non-zero exit code: %s", err)
	}

	log.Debugf("Finished executing command")
	return out, err
}
