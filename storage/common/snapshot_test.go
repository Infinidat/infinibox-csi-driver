//go:build unit

package common

import (
	"fmt"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/suite"
)

func (suite *SnapshotSuite) SetupTest() {
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{IboxAPI: suite.iboxapi, API: suite.api, AccessModesHelper: suite.accessMock}
}

type SnapshotSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

// validate snapshot locking expression
func (suite *SnapshotSuite) Test_Snapshot_Locking_Expression_Validation() {

	errString := "expected nil for valid lock_expires_at parameters"
	inParam := "1 Hours"
	futureTime, err := ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Days"
	futureTime, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Weeks"
	futureTime, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Months"
	futureTime, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	inParam = "1 Years"
	futureTime, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.Nil(suite.T(), err, errString)
	fmt.Printf("%s equates to future time of %v or unix millis %d\n", inParam, time.UnixMilli(futureTime), futureTime)

	// these next lock_expires parameters are invalid and should not validate
	inParam = "1 BadValue"
	_, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.NotNil(suite.T(), err, "expected not nil for valid lock_expires parameter")

	inParam = "X Years"
	_, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.NotNil(suite.T(), err, "expected not nil for valid lock_expires parameter")

	inParam = "1 Years a"
	_, err = ValidateSnapshotLockingParameter(time.Now().UnixMilli(), inParam)
	assert.NotNil(suite.T(), err, "expected not nil for valid lock_expires parameter")
}
