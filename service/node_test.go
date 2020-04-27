package service

import (
	"context"
	"infinibox-csi-driver/storage"
	"testing"

	"bou.ke/monkey"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type NodeTestSuite struct {
	suite.Suite
}

func TestNodeTestSuite(t *testing.T) {
	suite.Run(t, new(NodeTestSuite))
}

func (suite *NodeTestSuite) Test_NodePublishVolume_invalid_protocol() {
	nodePublishReq := getNodeNodePublishVolumeRequest()
	nodePublishReq.VolumeContext=map[string]string{"storage_protocol":"unknown"}
	s := getService()	
	_, err := s.NodePublishVolume(context.Background(), nodePublishReq)
	assert.NotNil(suite.T(), err, "storage_protocol value missing")	
}

func (suite *NodeTestSuite) Test_NodePublishVolume_success() {
	nodePublishReq := getNodeNodePublishVolumeRequest()
	s := getService()	
	patch := monkey.Patch(storage.NewStorageNode, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &NodeMock{}, nil
	})
	defer patch.Unpatch()
	
	_, err := s.NodePublishVolume(context.Background(), nodePublishReq)
	assert.Nil(suite.T(), err, "success")	
}


func (suite *NodeTestSuite) Test_NodeUnpublishVolume_invalid_protocol() {
	nodeUnPublishReq := getNodeUnpublishVolumeRequest()
	nodeUnPublishReq.VolumeId="100"
	s := getService()	
	_, err := s.NodeUnpublishVolume(context.Background(), nodeUnPublishReq)
	assert.NotNil(suite.T(), err, "storage_protocol value missing")	
}

func (suite *NodeTestSuite) Test_NodeUnpublishVolume_success() {
	nodeUnPublishReq := getNodeUnpublishVolumeRequest()
	s := getService()	
	patch := monkey.Patch(storage.NewStorageNode, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &NodeMock{}, nil
	})
	defer patch.Unpatch()
	_, err := s.NodeUnpublishVolume(context.Background(), nodeUnPublishReq)
	assert.Nil(suite.T(), err)	
}

func (suite *NodeTestSuite) Test_NodeGetCapabilities() {
	s := getService()	
	_, err := s.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	assert.Nil(suite.T(), err)	
}


func (suite *NodeTestSuite) Test_NodeGetInfo() {
	s := getService()	
	_, err := s.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
	assert.Nil(suite.T(), err)	
}
func (suite *NodeTestSuite) Test_NodeStageVolume_invalid_protocol() {
	nodeStageReq := getNodeStageVolumeRequest()
	nodeStageReq.VolumeContext=map[string]string{"storage_protocol":"unknown"}
	s := getService()	
	_, err := s.NodeStageVolume(context.Background(), nodeStageReq)
	assert.NotNil(suite.T(), err, "storage_protocol value missing")	
}

func (suite *NodeTestSuite) Test_NodeStageVolume_success() {
	nodeStageReq := getNodeStageVolumeRequest()
	s := getService()	
	patch := monkey.Patch(storage.NewStorageNode, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &NodeMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.NodeStageVolume(context.Background(), nodeStageReq)
	assert.Nil(suite.T(), err)	
}


func (suite *NodeTestSuite) Test_NodeUnstageVolume() {
	//s := getService()	
	//_, err := s.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{})
	//assert.Nil(suite.T(), err)	
}

func (suite *NodeTestSuite) Test_NodeGetVolumeStats() {
	s := getService()	
	_, err := s.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{})
	assert.NotNil(suite.T(), err)	
}



func (suite *NodeTestSuite) Test_NodeExpandVolume_invalid_ID() {
	nodeNodeExpandReq := getNodeExpandVolumeRequest()
	nodeNodeExpandReq.VolumeId=""	
	s := getService()	
	_, err := s.NodeExpandVolume(context.Background(), nodeNodeExpandReq)
	assert.NotNil(suite.T(), err, "Invalid ID")	
}



func (suite *NodeTestSuite) Test_NodeExpandVolume_success() {
	nodeNodeExpandReq := getNodeExpandVolumeRequest()	
	s := getService()	
	_, err := s.NodeExpandVolume(context.Background(), nodeNodeExpandReq)
	assert.Nil(suite.T(), err)	
}



//======================Data generator

func getNodeExpandVolumeRequest()*csi.NodeExpandVolumeRequest{
	return &csi.NodeExpandVolumeRequest{
		VolumeId: "100$$nfs",
	}
}
func getNodeStageVolumeRequest()*csi.NodeStageVolumeRequest{
	return &csi.NodeStageVolumeRequest{
		VolumeId: "100$$nfs",
		VolumeContext: map[string]string{"storage_protocol":"nfs"},
		Secrets:    getSecret(),
	}
}

func getNodeUnpublishVolumeRequest()*csi.NodeUnpublishVolumeRequest{
	return &csi.NodeUnpublishVolumeRequest{
		VolumeId: "100$$nfs",
	}
}
func getNodeNodePublishVolumeRequest() *csi.NodePublishVolumeRequest{
	return &csi.NodePublishVolumeRequest{
		VolumeId: "100$$nfs",	
		VolumeContext: map[string]string{"storage_protocol":"nfs"},
		Secrets:    getSecret(),
	}
}