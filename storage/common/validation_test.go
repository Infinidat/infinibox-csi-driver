//go:build unit

package common

import (
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/suite"
)

func (suite *ValidationSuite) SetupTest() {
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{IboxAPI: suite.iboxapi, API: suite.api, AccessModesHelper: suite.accessMock}
}

type ValidationSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestValidationSuite(t *testing.T) {
	suite.Run(t, new(ValidationSuite))
}

// test NFS and TREEQ protocal validate with network space.
func (suite *ValidationSuite) Test_Network_Protocol_Match_NFS_TREEQ_Success() {
	networkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceNFSService}
	var scProtocol = common.ProtocolNFS
	scNetSpace := []string{"someSpace", "someOtherSpace"}

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(networkSpace, nil)

	// validate NFS
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.Nil(suite.T(), err, "Expected Nil returned on success ")

	// validate TREEQ
	scProtocol = common.ProtocolTreeq
	err = ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.Nil(suite.T(), err, "Expected Nil returned on success ")
}

// validate iscsi protocol network space match
func (suite *ValidationSuite) Test_Network_Protocol_Match_ISCSI_Success() {
	scNetSpace := []string{"someSpace", "someOtherSpace"}

	// validate ISCSI
	scProtocol := common.ProtocolISCSI
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceISCSIService}
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.Nil(suite.T(), err, "Expected Nil returned on success ")
}

// validate iscsi protocol with NFS service fails
func (suite *ValidationSuite) Test_Network_Protocol_MisMatch_ISCSI_Failure() {
	scNetSpace := []string{"someSpace", "someOtherSpace"}

	// validate ISCSI
	scProtocol := common.ProtocolISCSI
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceNFSService}
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate NFS protocol with iSCSI service fails
func (suite *ValidationSuite) Test_Network_Protocol_MisMatch_NFS_Failure() {

	scNetSpace := []string{"someSpace", "someOtherSpace"}

	// validate ISCSI
	scProtocol := common.ProtocolNFS
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceISCSIService}
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate FC protocol with iSCSI service fails
func (suite *ValidationSuite) Test_Network_Protocol_MisMatch_FC_ISCSI_Failure() {
	scNetSpace := []string{"someSpace", "someOtherSpace"}

	scProtocol := common.ProtocolFC
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceISCSIService}
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate FC protocol with NFS service fails
func (suite *ValidationSuite) Test_Network_Protocol_MisMatch_FC_NFS_Failure() {
	scNetSpace := []string{"someSpace", "someOtherSpace"}

	scProtocol := common.ProtocolFC
	networkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceISCSIService}
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(networkSpace, nil)
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate iscsi protocol with iSCSI and NFS service fails
func (suite *ValidationSuite) Test_Network_Protocol_MisMatch_NAMESPACES_Failure() {
	scNetSpace := []string{"someiscsiSpace", "someNfsSpace"}

	scProtocol := common.ProtocolISCSI
	iscsiNetworkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceISCSIService}
	nfsNetworkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceNFSService}

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(iscsiNetworkSpace, nil).Once()
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(nfsNetworkSpace, nil).Once()
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate NFS protocol with no network spaces fails
func (suite *ValidationSuite) Test_Network_Protocol_NFS_NO_NETWORKSPACES_Failure() {
	scNetSpace := []string{} // no network spaces
	scProtocol := common.ProtocolFC
	networkSpace := &iboxapi.NetworkSpace{Service: common.NetworkSpaceISCSIService}

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(networkSpace, nil)
	err := ValidateProtocolToNetworkSpace(suite.Suite.T().Context(), scProtocol, scNetSpace, suite.cs.IboxAPI)
	assert.NotNil(suite.T(), err, "Expected non-nil for empty network space list")
}
