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
package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rexray/gocsi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"infinibox-csi-driver/api"
	"k8s.io/klog"
	"net"
	"os/exec"
	"strings"
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

func (s *service) validateStorageType(str string) (volprotoconf api.VolumeProtocolConfig, err error) {
	if str == "" {
		return volprotoconf, errors.New("volume Id empty - invalid")
	}
	volproto := strings.Split(str, "$$")
	if len(volproto) != 2 {
		return volprotoconf, errors.New("volume Id invalid")
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
