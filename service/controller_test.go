package service

import (
	"context"
	"errors"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func (suite *ControllerTestSuite) SetupTest() {
	suite.clientMock = new(MockClient)

}

type ControllerTestSuite struct {
	suite.Suite
	clientMock *MockClient
}

func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}

func (suite *ControllerTestSuite) Test_CreateVolume_Fail() {
	// Arrange

	expectedError := errors.New("Name cannot be empty")
	suite.clientMock.On("GetName()").Return("")
	service := service{apiclient: suite.clientMock}

	// Act
	_, err := service.CreateVolume(context.TODO(), getDummyparams())

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func getDummyparams() *csi.CreateVolumeRequest {
	param := csi.CreateVolumeRequest{}
	param.Name = "abc"
	return &param

}
