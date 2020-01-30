package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	csictx "github.com/rexray/gocsi/context"
	log "github.com/sirupsen/logrus"
)

const (
	Name                 = "infinibox-csi-driver"
	bytesInKiB           = 1024
	KeyThickProvisioning = "thickprovisioning"
	thinProvisioned      = "Thin"
	thickProvisioned     = "Thick"
)

var (
	NodeId           string = ""
	mergedParameters        = [...]string{
		0:  "fsType",
		1:  "targetPortal",
		2:  "portals",
		3:  "networkspace",
		4:  "readOnly",
		5:  "chapAuthDiscovery",
		6:  "chapAuthSession",
		7:  "iscsiInterface",
		8:  "networkspace",
		9:  "mountoptions",
		10: "lun",
	}
)

type storageoperations interface {
	csi.ControllerServer
	csi.NodeServer
}

type fcstorage struct {
	cs commonservice
}
type iscsistorage struct {
	cs commonservice
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
}

type commonservice struct {
	api               api.Client
	storagePoolIdName map[int64]string
	nodeID            string
	nodeIPAddress     string
}

// To return specific implementation of storage
func NewStorageController(storageType string, configparams ...map[string]interface{}) (storageoperations, error) {
	comnserv := buildCommonService(configparams[0])
	if storageType == "fc" {
		return &fcstorage{cs: comnserv}, nil
	} else if storageType == "iscsi" {
		return &iscsistorage{cs: comnserv}, nil
	} else if storageType == "nfs" {
		return &nfsstorage{cs: comnserv}, nil
	}
	return nil, errors.New("Error: Invalid storage type")
}

// To return specific implementation of storage
func NewStorageNode(storageType string, configparams ...map[string]interface{}) (storageoperations, error) {
	comnserv := buildCommonService(configparams[0])
	if storageType == "fc" {
		return &fcstorage{cs: comnserv}, nil
	} else if storageType == "iscsi" {
		return &iscsistorage{cs: comnserv}, nil
	} else if storageType == "nfs" {
		return &nfsstorage{cs: comnserv}, nil
	}
	return nil, errors.New("Error: Invalid storage type")
}

func buildCommonService(configParams map[string]interface{}) commonservice {
	commonService := commonservice{
		api: &api.ClientService{},
	}
	if configParams != nil {
		if configParams["nodeid"] == nil {
			log.Error("Validation Error: 'nodeid' is required field.")
		} else {
			commonService.nodeID = configParams["nodeid"].(string)
		}
		if configParams["nodeIPAddress"] == nil {
			log.Error("Validation Error: 'nodeIPAddress' is required field.")
		} else {
			commonService.nodeIPAddress = configParams["nodeIPAddress"].(string)
		}

	}
	err := commonService.verifyApiClient()
	if err != nil {
		log.Error("API client not initialized.", err)
	}
	log.Infoln("buildCommonService commonservice configuration done.")
	return commonService
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
	return &vols[0], nil
}

func (cs *commonservice) mapVolumeTohost(volumeID int) (luninfo api.LunInfo, err error) {
	host, err := cs.api.GetHostByName(cs.nodeID)
	if err != nil {
		return luninfo, err
	}
	luninfo, err = cs.api.MapVolumeToHost(host.ID, volumeID)
	if err != nil {
		return luninfo, err
	}
	return luninfo, nil
}

func (cs *commonservice) unMapVolumeFromhost(volumeID int) (err error) {
	host, err := cs.api.GetHostByName(cs.nodeID)
	if err != nil {
		return err
	}
	err = cs.api.UnMapVolumeFromHost(host.ID, volumeID)
	if err != nil {
		return err
	}
	return nil
}

func (cs *commonservice) deleteVolume(volumeID int) (err error) {
	err = cs.api.DeleteVolume(volumeID)
	if err != nil {
		return err
	}
	return nil
}

func (cs *commonservice) getCSIVolume(vol *api.Volume) *csi.Volume {
	log.Infof("getCSIVolume called with vol %v", vol)
	storagePoolName := vol.PoolName
	log.Infof("getCSIVolume storagePoolName is %s", vol.PoolName)
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
	}
	vi := &csi.Volume{
		VolumeId:      strconv.Itoa(vol.ID),
		CapacityBytes: vol.Size,
		VolumeContext: attributes,
	}
	return vi
}

// Convert an SIO Volume into a CSI Snapshot object suitable for return.
func (cs *commonservice) getCSISnapshot(vol *api.Volume) *csi.Snapshot {
	snapshot := &csi.Snapshot{
		SizeBytes:      int64(vol.Size) * bytesInKiB,
		SnapshotId:     strconv.Itoa(vol.ID),
		SourceVolumeId: strconv.Itoa(vol.ParentId),
	}
	// Convert array timestamp to CSI timestamp and add
	csiTimestamp, err := ptypes.TimestampProto(time.Unix(int64(vol.CreatedAt), 0))
	if err != nil {
		fmt.Printf("Could not convert time %v to ptypes.Timestamp %v\n", vol.CreatedAt, csiTimestamp)
	}
	if csiTimestamp != nil {
		snapshot.CreationTime = csiTimestamp
	}
	return snapshot
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
			log.Error("Could not found StoragePool: %d", id)
		}
	}
	return storagePoolName
}

func (cs *commonservice) getVolType(params map[string]string) string {
	volType := thinProvisioned
	if tp, ok := params[KeyThickProvisioning]; ok {
		tpb, err := strconv.ParseBool(tp)
		if err != nil {
			log.Error("invalid boolean received provision received params")
		} else if tpb {
			volType = thickProvisioned
		} else {
			volType = thinProvisioned
		}
	}

	return volType
}

//AddExportRule add export rule
func (cs *commonservice) AddExportRule(exportID, exportBlock, access, clientIPAdd string) (err error) {
	if exportID == "" || exportBlock == "" || clientIPAdd == "" {
		log.Errorf("invalid parameters")
		return errors.New("fail to add export rule")
	}
	exportId, _ := strconv.Atoi(exportID)
	_, err = cs.api.AddNodeInExport(exportId, access, false, clientIPAdd)
	if err != nil {
		log.Errorf("fail to add export rule %v", err)
		return
	}
	return nil
}

func DeleteExportRule(volumeID, clientIPAdd string) error {
	log.Errorf("######volumeID %v", volumeID)
	log.Errorf("#####clientIPAdd %v", clientIPAdd)

	return nil
}

func (cs *commonservice) getNetworkSpaceIP(config map[string]string) (string, error) {

	var networkSpace string
	networkSpace = strings.Trim(strings.Split(config["nfs_networkspace"], ",")[0], " ")

	nspace, err := cs.api.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return "", err
	}
	if len(nspace.Portals) == 0 {
		return "", errors.New("Ip address not found")
	}
	return nspace.Portals[0].IpAdress, nil
}
