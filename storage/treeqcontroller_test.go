package storage

import (
	"context"
	"infinibox-csi-driver/helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/suite"
)

func (suite *TreeqControllerSuite) SetupTest() {
	suite.osHelperMock = new(helper.MockOsHelper)
}

type TreeqControllerSuite struct {
	suite.Suite
	osHelperMock *helper.MockOsHelper
}

func TestTreeqControllerSuite(t *testing.T) {
	suite.Run(t, new(TreeqControllerSuite))
}

func (suite *TreeqControllerSuite) Test_CreateVolume_test() {
	service := treeqstorage{}
	service.CreateVolume(context.Background(), getTreeCreateVolumeRequest())
}

//Test data

func getTreeCreateVolumeRequest() *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{}

}
