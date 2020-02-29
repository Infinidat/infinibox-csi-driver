package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	"math/rand"
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
	Name                   = "infinibox-csi-driver"
	bytesInKiB             = 1024
	KeyThickProvisioning   = "thickprovisioning"
	thinProvisioned        = "Thin"
	thickProvisioned       = "Thick"
	KeyVolumeProvisionType = "provision_type"
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
}

type commonservice struct {
	api               api.Client
	storagePoolIdName map[int64]string
	initiatorPrefix   string
}

//NewStorageController : To return specific implementation of storage
func NewStorageController(storageProtocol string, configparams ...map[string]string) (storageoperations, error) {
	comnserv, err := buildCommonService(configparams[0], configparams[1])
	if err == nil {
		storageProtocol = strings.TrimSpace(storageProtocol)
		if storageProtocol == "fc" {
			return &fcstorage{cs: comnserv}, nil
		} else if storageProtocol == "iscsi" {
			return &iscsistorage{cs: comnserv}, nil
		} else if storageProtocol == "nfs" {
			return &nfsstorage{cs: comnserv, mounter: mount.New("")}, nil
		} else if storageProtocol == "nfs_treeq" {
			return &treeqstorage{filesysService: getFilesystemService(storageProtocol, comnserv), osHelper: helper.Service{}}, nil
		}
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
	return nil, err
}

//NewStorageNode : To return specific implementation of storage
func NewStorageNode(storageProtocol string, configparams ...map[string]string) (storageoperations, error) {
	comnserv, err := buildCommonService(nil, nil)
	if err == nil {
		storageProtocol = strings.TrimSpace(storageProtocol)
		if storageProtocol == "fc" {
			return &fcstorage{cs: comnserv}, nil
		} else if storageProtocol == "iscsi" {
			return &iscsistorage{cs: comnserv}, nil
		} else if storageProtocol == "nfs" {
			return &nfsstorage{cs: comnserv, mounter: mount.New("")}, nil
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
		commonserv.initiatorPrefix = config["initiatorPrefix"]
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

func (cs *commonservice) getAllHosts() ([]api.Host, error) {
	hosts, err := cs.api.GetAllHosts()
	if err != nil {
		log.Errorf("error get list of host with error: %v ", err)
		return nil, err
	}
	return hosts, nil
}

func (cs *commonservice) mapVolumeToAllhost(currentHostID, volumeID int, lun int) (err error) {
	hostList, err := cs.getAllHosts()
	if err != nil {
		return err
	}
	for _, hst := range hostList {
		if currentHostID != hst.ID {
			_, err = cs.api.MapVolumeToHost(hst.ID, volumeID, lun)
			if err != nil {
				if strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") || strings.Contains(err.Error(), "LUN_EXISTS") {
					continue
				}
				return err
			}
		}
	}
	return nil
}

func (cs *commonservice) unmapVolumeFromAllhost(volumeID int) (err error) {
	hostList, err := cs.getAllHosts()
	if err != nil {
		return err
	}
	for _, hst := range hostList {
		err = cs.api.UnMapVolumeFromHost(hst.ID, volumeID)
		if err != nil {
			// ignoring following error
			if strings.Contains(err.Error(), "HOST_NOT_FOUND") || strings.Contains(err.Error(), "LUN_NOT_FOUND") {
				continue
			}
			return err
		}
	}
	return nil
}

func (cs *commonservice) updateMappingForNewHost(hostID int) error {
	hostList, err := cs.getAllHosts()
	if err != nil {
		return err
	}
	luns := []api.LunInfo{}
	for _, hst := range hostList {
		if hst.ID != hostID {
			luns, err = cs.api.GetAllLunByHost(hst.ID)
			if err != nil {
				return err
			}
			break
		}
	}
	for _, lun := range luns {
		_, err = cs.api.MapVolumeToHost(hostID, lun.VolumeID, lun.Lun)
		if err != nil {
			if !strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
				return err
			}
		}
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
	}
	vi := &csi.Volume{
		VolumeId:      strconv.Itoa(vol.ID),
		CapacityBytes: vol.Size,
		VolumeContext: attributes,
		ContentSource: req.GetVolumeContentSource(),
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
			log.Errorf("Could not found StoragePool: %d", id)
		}
	}
	return storagePoolName
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
