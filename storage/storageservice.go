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
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	Name                 = "infinibox-csi-driver"
	bytesInKiB           = 1024
	KeyThickProvisioning = "thickprovisioning"
	thinProvisioned      = "Thin"
	thickProvisioned     = "Thick"
)

var (
	NodeId string = ""
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
	mounter      mount.Interface
}

type commonservice struct {
	api               api.Client
	storagePoolIdName map[int64]string
	nodeID            string
	nodeIPAddress     string
}

//NewStorageController : To return specific implementation of storage
func NewStorageController(storageProtocol string, configparams ...map[string]string) (storageoperations, error) {
	comnserv, err := buildCommonService(configparams[0], configparams[1])
	if err == nil {
		if storageProtocol == "fc" {
			return &fcstorage{cs: comnserv}, nil
		} else if storageProtocol == "iscsi" {
			return &iscsistorage{cs: comnserv}, nil
		} else if storageProtocol == "nfs" {
			return &nfsstorage{cs: comnserv, mounter: mount.New("")}, nil
		}
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
	return nil, err
}

//NewStorageNode : To return specific implementation of storage
func NewStorageNode(storageProtocol string, configparams ...map[string]string) (storageoperations, error) {
	comnserv, err := buildCommonService(nil, nil)
	if err == nil {
		if storageProtocol == "fc" {
			return &fcstorage{cs: comnserv}, nil
		} else if storageProtocol == "iscsi" {
			return &iscsistorage{cs: comnserv}, nil
		} else if storageProtocol == "nfs" {
			return &nfsstorage{cs: comnserv, mounter: mount.New("")}, nil
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
		} //TODO:
		if config["nodeid"] == "" {
			log.Error("Validation Error: 'nodeid' is required field.")
		} else {
			commonserv.nodeID = config["nodeid"]
		}
		if config["nodeIPAddress"] == "" {
			log.Error("Validation Error: 'nodeIPAddress' is required field.")
		} else {
			commonserv.nodeIPAddress = config["nodeIPAddress"]
		}
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

func (cs *commonservice) getCSIFsVolume(fsys *api.FileSystem) *csi.Volume {
	log.Infof("getCSIFsVolume called with fsys %v", fsys)
	storagePoolName := fsys.PoolName
	log.Infof("getCSIFsVolume storagePoolName is %s", fsys.PoolName)
	if storagePoolName == "" {
		storagePoolName = cs.getStoragePoolNameFromID(fsys.PoolID)
	}

	// Make the additional volume attributes
	attributes := map[string]string{
		"ID":              strconv.FormatInt(fsys.ID, 10),
		"Name":            fsys.Name,
		"StoragePoolID":   strconv.FormatInt(fsys.PoolID, 10),
		"StoragePoolName": storagePoolName,
		"CreationTime":    time.Unix(int64(fsys.CreatedAt), 0).String(),
	}

	vi := &csi.Volume{
		VolumeId:      strconv.FormatInt(fsys.ID, 10),
		CapacityBytes: fsys.Size,
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
	expID, _ := strconv.Atoi(exportID)
	_, err = cs.api.AddNodeInExport(expID, access, false, clientIPAdd)
	if err != nil {
		log.Errorf("fail to add export rule %v", err)
		return
	}
	return nil
}

// DeleteExportRule :
func DeleteExportRule(volumeID, clientIPAdd string) error {

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
