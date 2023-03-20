//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type IdentitySuite struct {
	suite.Suite
}

func TestIdentitySuite(t *testing.T) {
	suite.Run(t, new(IdentitySuite))
}

func (suite *IdentitySuite) Test_GetPluginInfo() {
	s := getService()
	_, err := s.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	assert.Nil(suite.T(), err)
}

func (suite *IdentitySuite) Test_GetPluginCapabilities() {
	s := getService()
	_, err := s.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	assert.Nil(suite.T(), err)
}

func (suite *IdentitySuite) Test_Probe() {
	s := getService()
	_, err := s.Probe(context.Background(), &csi.ProbeRequest{})
	assert.Nil(suite.T(), err)
}
