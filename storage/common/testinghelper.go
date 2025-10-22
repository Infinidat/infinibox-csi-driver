package common

import (
	"math/rand"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/mock"
	"k8s.io/mount-utils"
)

func GetVolume() *iboxapi.Volume {
	vol := iboxapi.Volume{
		ID:       100,
		PoolID:   10,
		ParentID: 1001,
		Name:     "volName",
		PoolName: "poolName",
		Size:     common.BytesInOneGibibyte,
	}
	return &vol
}

func GetSnapshotResp() *iboxapi.Snapshot {
	snap := &iboxapi.Snapshot{
		Name:       "snaName",
		SnapShotID: 1000,
		PoolID:     10,
	}
	return snap
}

func GetHostByName() *iboxapi.Host {
	var host iboxapi.Host
	host.ID = 10
	host.Name = "hostName"
	lunInfoArry := GetLunInfoArry()
	host.Luns = append(host.Luns, lunInfoArry...)
	var hostportArr []iboxapi.Ports
	var hostport iboxapi.Ports
	hostport.HostID = 10
	hostport.Address = "10.20.20.50"
	hostport.Type = "ISCSI"
	hostportArr = append(hostportArr, hostport)
	host.Ports = append(host.Ports, hostportArr...)
	return &host
}

func GetLunInfoArry() []iboxapi.LunInfo {
	var lunInfoArry []iboxapi.LunInfo
	lunInfoArry = append(lunInfoArry, GetLunInf())
	return lunInfoArry
}
func GetLunInf() iboxapi.LunInfo {
	lunInfo := iboxapi.LunInfo{
		HostID: 100,
		ID:     1,
	}
	return lunInfo
}

func GetSystem() *iboxapi.SystemDetails {
	sys := iboxapi.SystemDetails{
		SerialNumber: 1,
	}
	return &sys
}

func GetDeleteRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "103",
	}
}

func GetVolumeArray() []iboxapi.Volume {
	var volumes []iboxapi.Volume
	vol := GetVolume()
	volumes = append(volumes, *vol)
	return volumes
}

func GetNetworkspace() api.NetworkSpace {
	var nspace api.NetworkSpace
	var pArry []api.Portal
	portal := api.Portal{
		Enabled:     true,
		InterfaceID: 1,
		IPAddress:   "10.20.30.40",
		Reserved:    false,
		Tpgt:        100,
		Type:        "",
		VlanID:      100,
	}

	var netProp api.NetworkSpaceProperty
	netProp.ISCSIIqn = "iqn.1991-05.com.infinidate:example"

	nspace.Properties = netProp
	pArry = append(pArry, portal)
	nspace.Portals = append(nspace.Portals, pArry...)
	nspace.Service = common.NetworkSpaceISCSIService
	return nspace
}

// Test case Data Generation
func GetExportResponseValue() *iboxapi.Export {
	response := iboxapi.Export{ID: 1, ExportPath: "/exportPath/"}
	return &response
}

func GetExportPath() []iboxapi.Export {
	exportRepo := []iboxapi.Export{
		{ID: 1, ExportPath: "/exportPath/"},
	}

	return exportRepo
}

func GetExportResponseWithExports() (exportResp []iboxapi.Export) {
	exportRespArry := make([]iboxapi.Export, 1)

	exportRespArry[0] = iboxapi.Export{
		ExportPath: "/",
	}

	return exportRespArry
}

func GetExportResponse() []iboxapi.Export {
	ex := iboxapi.Export{}
	exportRespArry := []iboxapi.Export{}
	exportRespArry = append(exportRespArry, ex)
	return exportRespArry
}

func GetMetadataResponse() *[]api.Metadata {
	metadataArry := []api.Metadata{}
	return &metadataArry
}

func RandomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func GetFileSystemPrior() *iboxapi.FileSystem {
	return &iboxapi.FileSystem{
		ID:         1,
		PoolID:     100,
		Name:       "PVName",
		SSDEnabled: true,
		Provtype:   "thin",
		Size:       100 * GIB,
		PoolName:   "pool_name1",
	}
}
func GetFileSystem() *iboxapi.FileSystem {
	return &iboxapi.FileSystem{
		ID:         1,
		PoolID:     100,
		Name:       "PVName",
		SSDEnabled: true,
		Provtype:   "thin",
		Size:       100 * GIB,
		PoolName:   "pool_name1",
	}
}

func GetNetworkSpace() *iboxapi.NetworkSpace {
	portalArry := []iboxapi.Portal{{IPAddress: "10.20.20.50"}}
	return &iboxapi.NetworkSpace{Portals: portalArry, Service: common.NetworkSpaceNFSService}
}

func GetNodeUnPublishVolumeRequest(tagetPath string, volumeID string) *csi.NodeUnpublishVolumeRequest {
	return &csi.NodeUnpublishVolumeRequest{
		TargetPath: tagetPath,
		VolumeId:   volumeID,
	}
}

func GetNodePublishVolumeRequest(tagetPath string, publishContexMap map[string]string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		TargetPath:     tagetPath,
		PublishContext: publishContexMap,
	}
}

func GetNodePublishVolumeRequestReadonly(targetPath string, readonly bool, publishContexMap map[string]string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		TargetPath:     targetPath,
		Readonly:       readonly,
		PublishContext: publishContexMap,
	}
}

func GetPublishContexMap() map[string]string {
	contextMap := map[string]string{
		"ipAddress":                  "10.2.2.112",
		"volPathd":                   "/fs/filesytem/",
		"csiContainerHostMountPoint": "/host/",
	}
	return contextMap
}

func GetVolumeContexMap() map[string]string {
	contextMap := map[string]string{
		common.StorageClassNFSExportPermissions: "{'access':'RW','client':'*','no_root_squash':true}",
	}
	return contextMap
}

// MockNfsMounter - mount mock
type MockNfsMounter struct {
	mount.Interface
	mock.Mock
}

func (m *MockNfsMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	args := m.Called(file)
	resp, _ := args.Get(0).(bool)
	var err error
	if args.Get(1) == nil {
		err = nil
	} else {
		err, _ = args.Get(1).(error)
	}

	return resp, err
}

func (m *MockNfsMounter) Mount(source string, target string, fstype string, options []string) error {
	args := m.Called(source, target, fstype, options)
	var err error
	if args.Get(0) != nil {
		err = args.Get(0).(error)
	}
	return err
}

func (m *MockNfsMounter) Unmount(targetPath string) error {
	args := m.Called(targetPath)
	if args.Get(0) == nil {
		return nil
	}
	err := args.Get(0).(error)
	return err
}

type MockStorageHelper struct {
	mock.Mock
	StorageHelper
}

func (m *MockStorageHelper) SetVolumePermissions(req *csi.NodePublishVolumeRequest) error {
	status := m.Called(req)
	if status.Get(0) == nil {
		return nil
	}
	return status.Get(0).(error)
}
func (m *MockStorageHelper) ValidateIPAddress(ip string, port int) error {
	status := m.Called(ip, port)
	if status.Get(0) == nil {
		return nil
	}
	return status.Get(0).(error)
}

func (m *MockStorageHelper) GetNFSMountOptions(req *csi.NodePublishVolumeRequest) ([]string, error) {
	status := m.Called(req)
	if status.Get(1) == nil {
		return []string{}, nil
	}
	return status.Get(0).([]string), status.Get(1).(error)
}

func GetIboxapiCreateVolumeResponse() *iboxapi.Volume {
	vol := iboxapi.Volume{
		ID:       100,
		PoolID:   10,
		ParentID: 1001,
		Name:     "volName",
		PoolName: "poolName",
		Size:     common.BytesInOneGibibyte,
	}
	return &vol
}

// TODO refactor protocol out of these test functions
func GetISCSIControllerPublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId:      "1$$iscsi",
		NodeId:        "10.20.20.50$$iscsi",
		VolumeContext: map[string]string{common.StorageClassMaxVolsPerHost: "10"},
	}
}
func GetISCSIControllerUnpublishVolume() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "1$$nfs",
		NodeId:   "10.20.20.50$$iscsi",
	}
}

func GetISCSIExpandVolumeRequest() *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: "1",
	}
}

func GetISCSIDeleteSnapshotRequest() *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: "1$$iscsi",
	}
}

func GetISCSICreateSnapshotRequest() *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: "1$$iscsi",
		Name:           "snapshotName",
	}
}
