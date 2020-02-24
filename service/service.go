package service

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"net"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rexray/gocsi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ServiceName = "infinibox-csi-driver"
)

type service struct {
	//service
	apiclient api.Client

	// parameters
	config              ServiceConfig
	mode                string
	storagePoolIDToName map[int64]string
	nodeID              string
	maxVolumesPerNode   int64
	driverName          string
	nodeIPAddress       string
	blockMountDir       string
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
		blockMountDir:       configParam["blockMountDir"],
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

func (s *service) validateStorageType(str string) (volprotoconf api.VolumeProtocolConfig, err error) {
	volproto := strings.Split(str, "$$")
	if len(volproto) != 2 {
		return volprotoconf, errors.New("volume Id and other details not found")
	}
	log.Info("volproto ", volproto)
	volprotoconf.VolumeID = volproto[0]
	volprotoconf.StorageType = volproto[1]
	return volprotoconf, nil
}

// Controller expand volume request validation
func (s *service) validateExpandVolumeRequest(req *csi.ControllerExpandVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}

	return nil
}
