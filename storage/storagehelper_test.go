//go:build unit

package storage

import (
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/suite"
)

func (suite *StorageHelperSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.iboxapi = new(iboxapi.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{IboxApi: suite.iboxapi, Api: suite.api, AccessModesHelper: suite.accessMock}
}

type StorageHelperSuite struct {
	suite.Suite
	api        *api.MockApiService
	iboxapi    *iboxapi.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestStorageHelperSuite(t *testing.T) {
	suite.Run(t, new(StorageHelperSuite))
}

// test NFS and TREEQ protocal validate with network space.
func (suite *StorageHelperSuite) Test_Network_Protocol_Match_NFS_TREEQ_Success() {

	networkSpace := &iboxapi.NetworkSpace{Service: common.NS_NFS_SVC}
	var scProtocol = common.PROTOCOL_NFS
	scNetSpace := []string{"someSpace", "someOtherSpace"}

	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)

	// validate NFS
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.Nil(suite.T(), err, "Expected Nil returned on success ")

	// validate TREEQ
	scProtocol = common.PROTOCOL_TREEQ
	err = ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.Nil(suite.T(), err, "Expected Nil returned on success ")

}

// validate iscsi protocol network space match
func (suite *StorageHelperSuite) Test_Network_Protocol_Match_ISCSI_Success() {

	scNetSpace := []string{"someSpace", "someOtherSpace"}

	// validate ISCSI
	scProtocol := common.PROTOCOL_ISCSI
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NS_ISCSI_SVC}
	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.Nil(suite.T(), err, "Expected Nil returned on success ")
}

// validate iscsi protocol with NFS service fails
func (suite *StorageHelperSuite) Test_Network_Protocol_MisMatch_ISCSI_Failure() {

	scNetSpace := []string{"someSpace", "someOtherSpace"}

	// validate ISCSI
	scProtocol := common.PROTOCOL_ISCSI
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NS_NFS_SVC}
	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate NFS protocol with iSCSI service fails
func (suite *StorageHelperSuite) Test_Network_Protocol_MisMatch_NFS_Failure() {

	scNetSpace := []string{"someSpace", "someOtherSpace"}

	// validate ISCSI
	scProtocol := common.PROTOCOL_NFS
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NS_ISCSI_SVC}
	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate FC protocol with iSCSI service fails
func (suite *StorageHelperSuite) Test_Network_Protocol_MisMatch_FC_ISCSI_Failure() {

	scNetSpace := []string{"someSpace", "someOtherSpace"}

	scProtocol := common.PROTOCOL_FC
	iNetworkSpace := &iboxapi.NetworkSpace{Service: common.NS_ISCSI_SVC}
	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(iNetworkSpace, nil)
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate FC protocol with NFS service fails
func (suite *StorageHelperSuite) Test_Network_Protocol_MisMatch_FC_NFS_Failure() {

	scNetSpace := []string{"someSpace", "someOtherSpace"}

	scProtocol := common.PROTOCOL_FC
	networkSpace := &iboxapi.NetworkSpace{Service: common.NS_ISCSI_SVC}
	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate iscsi protocol with iSCSI and NFS service fails
func (suite *StorageHelperSuite) Test_Network_Protocol_MisMatch_NAMESPACES_Failure() {

	scNetSpace := []string{"someiscsiSpace", "someNfsSpace"}

	scProtocol := common.PROTOCOL_ISCSI
	iscsiNetworkSpace := &iboxapi.NetworkSpace{Service: common.NS_ISCSI_SVC}
	nfsNetworkSpace := &iboxapi.NetworkSpace{Service: common.NS_NFS_SVC}

	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(iscsiNetworkSpace, nil).Once()
	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(nfsNetworkSpace, nil).Once()
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.NotNil(suite.T(), err, "Expected iscsi to not match with NFS service ")
}

// validate NFS protocol with no network spaces fails
func (suite *StorageHelperSuite) Test_Network_Protocol_NFS_NO_NETWORKSPACES_Failure() {

	scNetSpace := []string{} // no network spaces
	scProtocol := common.PROTOCOL_FC
	networkSpace := &iboxapi.NetworkSpace{Service: common.NS_ISCSI_SVC}

	suite.iboxapi.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)
	err := ValidateProtocolToNetworkSpace(scProtocol, scNetSpace, suite.cs.IboxApi)
	assert.NotNil(suite.T(), err, "Expected non-nil for empty network space list")
}

// validate snapshot locking expression
func (suite *StorageHelperSuite) Test_Snapshot_Locking_Expression_Validation() {

	errString := "expected nil for valid lock_expires_at parameters"
	inParam := "1 Hours"
	futureTime, err := validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Days"
	futureTime, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Weeks"
	futureTime, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Months"
	futureTime, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Years"
	futureTime, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	// these next lock_expires parameters are invalid and should not validate
	inParam = "1 BadValue"
	_, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.NotNil(suite.T(), err, "expected not nil for valid lock_expires parameter")

	inParam = "X Years"
	_, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.NotNil(suite.T(), err, "expected not nil for valid lock_expires parameter")

	inParam = "1 Years a"
	_, err = validateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.NotNil(suite.T(), err, "expected not nil for valid lock_expires parameter")
}
