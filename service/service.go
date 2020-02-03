package service

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"github.com/rexray/gocsi"
	csictx "github.com/rexray/gocsi/context"
	log "github.com/sirupsen/logrus"
)

const (
	Name                 = "infinibox-csi-driver"
	bytesInKiB           = 1024
	KeyThickProvisioning = "thickprovisioning"
	thinProvisioned      = "Thin"
	thickProvisioned     = "Thick"

	// env variables

)

var (
	mergedParameters = [...]string{
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

type service struct {
	//service
	apiclient api.Client

	// parameters
	config              ServiceConfig
	mode                string
	volCache            []*api.Volume
	volCacheRWL         sync.RWMutex
	snapCache           []*api.Volume
	snapCacheRWL        sync.RWMutex
	sdcMap              map[string]string
	sdcMapRWL           sync.RWMutex
	spCache             map[string]int
	spCacheRWL          sync.RWMutex
	privDir             string
	storagePoolIDToName map[int64]string
	statisticsCounter   int
	nodeID              string
	maxVolumesPerNode   int64
	driverName          string
	nodeIPAddress       string
}

// Config defines service configuration options.
type ServiceConfig struct {
	EndPoint                   string
	UserName                   string
	Password                   string
	max_fs                     string
	PoolName                   string
	Thick                      bool
	ssd_enabled                string
	provision_type             string
	Insecure                   bool
	DisableCerts               bool   // used for unit testing only
	Lsmod                      string // used for unit testing only
	EnableSnapshotCGDelete     bool   // when snapshot deleted, enable deleting of all snaps in the CG of the snapshot
	EnableListVolumesSnapshots bool   // when listing volumes, include snapshots and volumes

}

// Service is the CSI Mock service provider.
type Service interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer

	BeforeServe(context.Context, *gocsi.StoragePlugin, net.Listener) error
}

// New returns a new Service.
func New(configParam map[string]string) Service {
	return &service{
		nodeID:              configParam["nodeid"],
		driverName:          configParam["drivername"],
		nodeIPAddress:       configParam["nodeIPAddress"],
		sdcMap:              map[string]string{},
		spCache:             map[string]int{},
		storagePoolIDToName: map[int64]string{},
		apiclient:           &api.ClientService{},
	}
}

func (s *service) BeforeServe(ctx context.Context, sp *gocsi.StoragePlugin, listner net.Listener) error {
	s.verifyController()
	return nil
}

func (s *service) verifyController() error {
	if s.apiclient == nil {
		c, err := s.apiclient.NewClient()
		if err != nil {
			return errors.New("failed to create rest client")
		}
		s.apiclient = c
	}
	return nil
}

func (s *service) getIscsiInitiatorName() string {
	if ep, ok := csictx.LookupEnv(context.Background(), "ISCSI_INITIATOR_NAME"); ok {
		return ep
	}
	return ""
}
func (s *service) getVolumeByID(id int) (*api.Volume, error) {

	// The `GetVolume` API returns a slice of volumes, but when only passing
	// in a volume ID, the response will be just the one volume
	vols, err := s.apiclient.GetVolume(id)
	if err != nil {
		return nil, err
	}
	return &vols[0], nil
}

func (s *service) mapVolumeTohost(volumeID int) (luninfo api.LunInfo, err error) {
	host, err := s.apiclient.GetHostByName(s.nodeID)
	if err != nil {
		return luninfo, err
	}
	luninfo, err = s.apiclient.MapVolumeToHost(host.ID, volumeID)
	if err != nil {
		return luninfo, err
	}
	return luninfo, nil
}

func (s *service) unMapVolumeFromhost(volumeID int) (err error) {
	host, err := s.apiclient.GetHostByName(s.nodeID)
	if err != nil {
		return err
	}
	err = s.apiclient.UnMapVolumeFromHost(host.ID, volumeID)
	if err != nil {
		return err
	}
	return nil
}

func (s *service) deleteVolume(volumeID int) (err error) {
	err = s.apiclient.DeleteVolume(volumeID)
	if err != nil {
		return err
	}
	return nil
}

func (s *service) getCSIVolume(vol *api.Volume) *csi.Volume {
	// Get storage pool name; add to cache of ID to Name if not present
	storagePoolName := vol.PoolName
	if storagePoolName == "" {
		storagePoolName = s.getStoragePoolNameFromID(vol.PoolId)
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
func (s *service) getCSISnapshot(vol *api.Volume) *csi.Snapshot {
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

func (s *service) getStoragePoolNameFromID(id int64) string {
	storagePoolName := s.storagePoolIDToName[id]
	if storagePoolName == "" {
		pool, err := s.apiclient.FindStoragePool(id, "")
		if err == nil {
			storagePoolName = pool.Name
			s.storagePoolIDToName[id] = pool.Name
		} else {
			log.Error("Could not found StoragePool: %d", id)
		}
	}
	return storagePoolName
}

// Provide periodic logging of statistics like goroutines and memory
func (s *service) logStatistics() {
	if s.statisticsCounter = s.statisticsCounter + 1; (s.statisticsCounter % 100) == 0 {
		goroutines := runtime.NumGoroutine()
		memstats := new(runtime.MemStats)
		runtime.ReadMemStats(memstats)
		fields := map[string]interface{}{
			"GoRoutines":   goroutines,
			"HeapAlloc":    memstats.HeapAlloc,
			"HeapReleased": memstats.HeapReleased,
			"StackSys":     memstats.StackSys,
		}
		log.WithFields(fields).Info("statistics")
	}
}

func (s *service) clearCache() {
	s.volCacheRWL.Lock()
	defer s.volCacheRWL.Unlock()
	s.volCache = make([]*api.Volume, 0)
	s.snapCacheRWL.Lock()
	defer s.snapCacheRWL.Unlock()
	s.snapCache = make([]*api.Volume, 0)
}

// getVolProvisionType returns a string indicating thin or thick provisioning
// If the type is specified in the params map, that value is used, if not, defer
// to the service config
func (s *service) getVolProvisionType(params map[string]string) string {
	volType := thinProvisioned
	if s.config.Thick {
		volType = thickProvisioned
	}

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
