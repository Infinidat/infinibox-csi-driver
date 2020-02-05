package service

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
	log "github.com/sirupsen/logrus"
)

// Manifest is the SP's manifest.
var Manifest = map[string]string{
	"url":    "http://github.com/infinidat/csi-infinidat-driver",
	"semver": "1.0.0",
	"commit": "",
	"formed": "",
}

func (s *service) GetPluginInfo(
	ctx context.Context,
	req *csi.GetPluginInfoRequest) (
	*csi.GetPluginInfoResponse, error) {

	return &csi.GetPluginInfoResponse{
		Name:          s.driverName,
		VendorVersion: "1.0.0",
	}, nil
}

func (s *service) GetPluginCapabilities(
	ctx context.Context,
	req *csi.GetPluginCapabilitiesRequest) (
	*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
                        {
                                Type: &csi.PluginCapability_Service_{
                                        Service: &csi.PluginCapability_Service{
                                                Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
                                        },
                                },
                        },
		},
	}, nil
}

func (s *service) Probe(
	ctx context.Context,
	req *csi.ProbeRequest) (
	*csi.ProbeResponse, error) {

	ready := new(wrappers.BoolValue)
	ready.Value = true
	proberes := new(csi.ProbeResponse)
	proberes.Ready = ready
	log.Debugf("Probe returning: %v", proberes.Ready.GetValue())

	return proberes, nil
}
