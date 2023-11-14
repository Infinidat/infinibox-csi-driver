//go:build unit

package service

import (
	"context"
	"infinibox-csi-driver/helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	DefaultDriverName = "infinibox-csi-driver"
)

type IdentitySuite struct {
	suite.Suite
}

func TestIdentitySuite(t *testing.T) {
	suite.Run(t, new(IdentitySuite))
}

func (suite *IdentitySuite) TestGetPluginCapabilities() {
	expectedCap := []*csi.PluginCapability{
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
	}

	d := NewEmptyDriver("")
	fakeIdentityServer := IdentityServer{
		Driver: d,
	}
	req := csi.GetPluginCapabilitiesRequest{}
	resp, err := fakeIdentityServer.GetPluginCapabilities(context.Background(), &req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.Equal(suite.T(), resp.XXX_sizecache, int32(0))
	assert.Equal(suite.T(), resp.Capabilities, expectedCap)

}

func (suite *IdentitySuite) TestProbe() {
	d := NewEmptyDriver("")
	req := csi.ProbeRequest{}
	fakeIdentityServer := IdentityServer{
		Driver: d,
	}
	resp, err := fakeIdentityServer.Probe(context.Background(), &req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.Equal(suite.T(), resp.XXX_sizecache, int32(0))
	assert.Equal(suite.T(), resp.Ready.Value, true)
}

func (suite *IdentitySuite) TestGetPluginInfo() {
	req := csi.GetPluginInfoRequest{}
	emptyNameDriver := NewEmptyDriver("name")
	emptyVersionDriver := NewEmptyDriver("version")
	tests := []struct {
		desc        string
		driver      *Driver
		expectedErr error
	}{
		{
			desc:        "Successful Request",
			driver:      NewEmptyDriver(""),
			expectedErr: nil,
		},
		{
			desc:        "Driver name missing",
			driver:      emptyNameDriver,
			expectedErr: status.Error(codes.Unavailable, "Driver name not configured"),
		},
		{
			desc:        "Driver version missing",
			driver:      emptyVersionDriver,
			expectedErr: status.Error(codes.Unavailable, "Driver is missing version"),
		},
	}

	for _, test := range tests {
		fakeIdentityServer := IdentityServer{
			Driver: test.driver,
		}
		_, err := fakeIdentityServer.GetPluginInfo(context.Background(), &req)
		//if !reflect.DeepEqual(err, test.expectedErr) {
		if err == nil && test.expectedErr != nil {
			suite.T().Errorf("Unexpected error: %v\nExpected: %v", err, test.expectedErr)
		}
		if err != nil && test.expectedErr == nil {
			suite.T().Errorf("Unexpected error: %v\nExpected: %v", err, test.expectedErr)
		}
		if err != nil && test.expectedErr != nil {
			if err.Error() != test.expectedErr.Error() {
				suite.T().Errorf("Unexpected error: %v\nExpected: %v", err, test.expectedErr)
			}
		}
	}
}

func NewEmptyDriver(emptyField string) *Driver {
	var d *Driver
	switch emptyField {
	case "version":
		d = &Driver{
			name:    DefaultDriverName,
			version: "",
			nodeID:  fakeNodeID,
		}
	case "name":
		d = &Driver{
			name:    "",
			version: "n/a",
			nodeID:  fakeNodeID,
		}
	default:
		d = &Driver{
			name:    DefaultDriverName,
			version: "n/a",
			nodeID:  fakeNodeID,
		}
	}
	d.volumeLocks = helper.NewVolumeLocks()
	return d
}

const (
	fakeNodeID = "fakeNodeID"
)
