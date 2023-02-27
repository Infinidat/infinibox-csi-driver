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
package service

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"net"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rexray/gocsi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	ServiceName = "infinibox-csi-driver"
)

type service struct {
	// service
	apiclient api.Client
	// parameters
	nodeID        string
	nodeName      string
	driverName    string
	driverVersion string
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
		apiclient: &api.ClientService{},
		// parameters
		nodeID:        configParam["nodeid"],
		nodeName:      configParam["nodeName"],
		driverName:    configParam["drivername"],
		driverVersion: configParam["driverversion"],
	}
}

func (s *service) BeforeServe(ctx context.Context, sp *gocsi.StoragePlugin, listener net.Listener) error {
	return s.verifyController()
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
		klog.Warningf("could not get fqdn with cmd : 'hostname -f', get hostname with 'echo $HOSTNAME'")
		cmd = "echo $HOSTNAME"
		out, err = exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			klog.Errorf("Failed to execute command: %s", cmd)
			return s.nodeName
		}
	}
	nodeFQDN := string(out)
	if nodeFQDN == "" {
		klog.Warningf("node fqnd not found, setting node name as node fqdn instead")
		nodeFQDN = s.nodeName
	}
	nodeFQDN = strings.TrimSuffix(nodeFQDN, "\n")
	return nodeFQDN
}

func (s *service) validateNodeID(nodeID string) error {
	if nodeID == "" {
		return status.Error(codes.InvalidArgument, "node ID empty")
	}
	nodeSplit := strings.Split(nodeID, "$$")
	if len(nodeSplit) != 2 {
		return status.Error(codes.NotFound, "node Id does not follow '<fqdn>$$<id>' pattern")
	}
	return nil
}

func (s *service) validateVolumeID(str string) (volprotoconf api.VolumeProtocolConfig, err error) {
	if str == "" {
		return volprotoconf, status.Error(codes.InvalidArgument, "volume Id empty")
	}
	volproto := strings.Split(str, "$$")
	if len(volproto) != 2 {
		return volprotoconf, status.Error(codes.NotFound, "volume Id does not follow '<id>$$<proto>' pattern")
	}
	klog.V(2).Infof("volproto: %s", volproto)
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
