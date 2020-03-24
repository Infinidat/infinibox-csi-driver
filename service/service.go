package service

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"net"
	"os/exec"
	"strings"
	//"sync"

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
	mode                string
	storagePoolIDToName map[int64]string
	nodeID              string
	maxVolumesPerNode   int64
	driverName          string
	driverVersion       string
	nodeIPAddress       string
	nodeName            string
	initiatorPrefix     string
	hostclustername     string
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
		nodeName:            configParam["nodeName"],
		initiatorPrefix:     configParam["initiatorPrefix"],
		hostclustername:     configParam["hostclustername"],
		driverVersion:       configParam["driverversion"],
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

func (s *service) getNodeFQDN() string {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from getNodeFQDN  " + fmt.Sprint(res))
		}
	}()
	cmd := "hostname -f"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Errorf("Failed to execute command: %s", cmd)
		return s.nodeName
	}
	nodeFQDN := string(out)
	if nodeFQDN == "" {
		log.Warning("node fqnd not found, setting node name as node fqdn instead")
		nodeFQDN = s.nodeName
	}
	nodeFQDN = strings.TrimSuffix(nodeFQDN, "\n")
	return nodeFQDN
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
