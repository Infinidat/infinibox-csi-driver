package storage

import (
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *FileSystemServiceSuite) SetupTest() {
	suite.nfsMountMock = new(MockNfsMounter)
	suite.api = new(api.MockApiService)
	suite.cs = &commonservice{api: suite.api}
}

type FileSystemServiceSuite struct {
	suite.Suite
	nfsMountMock *MockNfsMounter
	api          *api.MockApiService
	cs           *commonservice
}

func TestFileSystemServiceSuite(t *testing.T) {
	suite.Run(t, new(FileSystemServiceSuite))
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_GetPoolIDByPoolName_error() {
	expectedErr := errors.New("some error")
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(0, expectedErr)
	service := FilesystemService{mounter: suite.nfsMountMock, cs: *suite.cs}
	_, err := service.getExpectedFileSystemID()
	assert.NotNil(suite.T(), err, "empty object")

}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_getMaxSize_error() {
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	service := FilesystemService{mounter: suite.nfsMountMock, cs: *suite.cs}
	configmap := make(map[string]string)
	configmap[MAXFILESYSTEMSIZE] = "4mib"
	service.configmap = configmap
	_, err := service.getExpectedFileSystemID()
	fmt.Println(err)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_FileSystemByPoolID_error() {
	expectedErr := errors.New("some error")
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(nil, expectedErr)
	service := FilesystemService{mounter: suite.nfsMountMock, cs: *suite.cs}
	_, err := service.getExpectedFileSystemID()
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_FilesytemTreeqCount_error() {
	expectedErr := errors.New("some error")
	fsMetada := getfsMetadata2()
	var poolID int64 = 10
	var fsID int64 = 11
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(*fsMetada, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(0, expectedErr)
	service := FilesystemService{mounter: suite.nfsMountMock, cs: *suite.cs}
	_, err := service.getExpectedFileSystemID()
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_Success() {
	fsMetada := getfsMetadata()
	var poolID int64 = 10
	var fsID int64 = 11
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(*fsMetada, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(1, nil)

	exportResp := getExportResponse()
	suite.api.On("GetExportByFileSystem", fsID).Return(exportResp, nil)
	fsMetada2 := getfsMetadata2()
	suite.api.On("GetFileSystemsByPoolID", poolID, 2).Return(*fsMetada2, nil)
	service := FilesystemService{mounter: suite.nfsMountMock, cs: *suite.cs}
	/** paramter values to filesystemService  */
	service.capacity = 1000
	service.exportpath = "/exportPath"

	fs, err := service.getExpectedFileSystemID()
	assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), fs.ID, fsID, "file system ID equal")
}

func (suite *FileSystemServiceSuite) Test_CreateNFSVolume_Success() {
	fsMetada := getfsMetadata2()
	var poolID int64 = 10
	var fsID int64 = 11
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(*fsMetada, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(1, nil)

	exportResp := getExportResponse()
	suite.api.On("GetExportByFileSystem", fsID).Return(exportResp, nil)

	treeqResp := getTreeQResponse(fsID)
	suite.api.On("CreateTreeq", fsID, mock.Anything).Return(*treeqResp, nil)

	metadataResp := getMetadaResponse()
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(*metadataResp, nil)

	suite.api.On("UpdateFilesystem", fsID, mock.Anything).Return(nil, nil)
	service := FilesystemService{mounter: suite.nfsMountMock, cs: *suite.cs}
	/** paramter values to filesystemService  */
	service.capacity = 1000
	service.pVName = "csi-TestTreeq"
	service.exportpath = "/exportPath"
	service.treeqVolume = make(map[string]string)

	err := service.CreateNFSVolume()
	assert.Nil(suite.T(), err, "empty object")

}

//*****Test case Data Generation

func getExportResponse() *[]api.ExportResponse {
	exportRespArry := []api.ExportResponse{}

	exportResp := api.ExportResponse{}
	exportResp.ExportPath = "/exportPath"
	return &exportRespArry
}
func getMetadaResponse() *[]api.Metadata {
	metadataArry := []api.Metadata{}
	return &metadataArry
}
func getTreeQResponse(fileSysID int64) *api.Treeq {
	var treeq api.Treeq
	treeq.FilesystemID = fileSysID
	treeq.HardCapacity = 1000
	treeq.ID = 1
	treeq.Name = "csi-TestTreeq"
	treeq.Path = "/csi-TestTreeq"
	return &treeq
}

func getCreateVolumeRequest( /*tagetPath string, publishContexMap map[string]string*/ ) *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{}
}

func getfsMetadata() *api.FSMetadata {
	var fsMetadata api.FSMetadata
	fs := api.FileSystem{}
	fs.ID = 10
	fs.Size = 1073741824
	fsArry := []api.FileSystem{}
	fsArry = append(fsArry, fs)
	fsMeta := api.FileSystemMetaData{}
	fsMeta.NumberOfObjects = 1
	fsMeta.Page = 1
	fsMeta.PageSize = 50
	fsMeta.PagesTotal = 2
	fsMeta.Ready = true

	fsMetadata.Filemetadata = fsMeta
	fsMetadata.FileSystemArry = fsArry
	return &fsMetadata
}

func getfsMetadata2() *api.FSMetadata {
	var fsMetadata api.FSMetadata
	fs := api.FileSystem{}
	fs.ID = 11
	fs.Size = 10000
	fsArry := []api.FileSystem{}
	fsArry = append(fsArry, fs)
	fsMeta := api.FileSystemMetaData{}
	fsMeta.NumberOfObjects = 1
	fsMeta.Page = 2
	fsMeta.PageSize = 50
	fsMeta.PagesTotal = 2
	fsMeta.Ready = true

	fsMetadata.Filemetadata = fsMeta
	fsMetadata.FileSystemArry = fsArry
	return &fsMetadata
}
