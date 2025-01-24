/*
Copyright 2025 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package iboxapi

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
)

const (
	TRACE_LEVEL = 2
	DEBUG_LEVEL = 1
	INFO_LEVEL  = 0
)

var ERROR_CODE_HOST_NOT_FOUND = "HOST_NOT_FOUND"

var ErrNotFound = errors.New("resource not found")

type Metadata struct {
	Ready           bool `json:"ready"`
	NumberOfObjects int  `json:"number_of_objects"`
	PageSize        int  `json:"page_size"`
	PagesTotal      int  `json:"pages_total"`
	Page            int  `json:"page"`
}

type Error struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Reasons  []any  `json:"reasons"`
	Severity string `json:"severity"`
	IsRemote bool   `json:"is_remote"`
	Data     any    `json:"data"`
}

// Client interface
type Client interface {
	// pools
	GetPoolByName(name string) (*PoolResult, error)

	/**
	NewClient() (*ClientService, error)
	CreateVolume(volume *VolumeParam, storagePoolName string) (*Volume, error)
	GetStoragePoolIDByName(name string) (id int64, err error)
	FindStoragePool(id int64, name string) (StoragePool, error)
	GetNtpStatus() ([]NtpStatus, error)
	GetStoragePool(poolID int64, storagepool string) ([]StoragePool, error)
	GetVolumeByName(volumename string) (*Volume, error)
	GetVolume(volumeid int) (*Volume, error)
	CreateSnapshotVolume(lockExpiresAt int64, snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error)
	GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error)
	DeleteVolume(volumeID int) (err error)
	UpdateVolume(volumeID int, volume Volume) (*Volume, error)
	GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error)
	GetAllSnapshots() ([]Volume, error)
	GetAllVolumes() ([]Volume, error)
	*/

	// hosts
	GetAllHosts() (host []Host, err error)
	GetHostByName(hostName string) (host *Host, err error)
	CreateHost(hostName string) (host *CreateHostResponse, err error)
	DeleteHost(hostID int) (resp *DeleteHostResponse, err error)
	AddHostSecurity(chapCreds map[string]string, hostID int) (host *AddHostSecurityResponse, err error)
	AddHostPort(portType, portAddress string, hostID int) (addPortResponse *AddPortResponse, err error)
	GetHostPort(hostID int, portAddress string) (hostPort *HostPort, err error)
	MapVolumeToHost(hostID, volumeID, lun int) (lunInfo *LunInfo, err error)
	GetAllLunByHost(hostID int) (luninfo []Luns, err error)
	GetLunByHostVolume(hostID, volumeID int) (lun *Luns, err error)
	UnMapVolumeFromHost(hostID, volumeID int) (resp *UnMapVolumeFromHostResponse, err error)

	// volumes
	GetLunsByVolume(volumeID int) (resp []Luns, err error)

	/**
	GetFCPorts() (fcNodes []FCNode, err error)

	// for consistency group (volume group)
	CreateCG(poolID int, cgName string) (CGInfo, error)
	AddMemberToSnapshotGroup(volumeID int, cgID int) error
	RemoveMemberFromSnapshotGroup(volumeID int, cgID int) error
	GetAllCG() ([]CGInfo, error)
	GetMembersByCGID(cgID int) ([]MemberInfo, error)
	GetCG(name string) (CGInfo, error)
	CreateSnapshotGroup(cgID int, snapName, snapPrefix, snapSuffix string) (CGInfo, error)

	// for nfs
	ExportFileSystem(export ExportFileSys) (*ExportResponse, error)
	DeleteExportPath(exportID int64) (*ExportResponse, error)
	DeleteFileSystem(fileSystemID int64) (*FileSystem, error)
	AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error)
	DetachMetadataFromObject(objectID int64) (*[]Metadata, error)
	CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error)
	GetExportByFileSystem(filesystemID int64) (*[]ExportResponse, error)
	AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	DeleteNodeFromExport(exportID int64, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	CreateFileSystemSnapshot(lockedExpiresAt int64, snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponce, error)
	DeleteFileSystemComplete(fileSystemID int64) (err error)
	DeleteParentFileSystem(fileSystemID int64) (err error)
	GetParentID(fileSystemID int64) int64
	GetFileSystemByID(fileSystemID int64) (*FileSystem, error)
	GetFileSystemByName(fileSystemName string) (*FileSystem, error)
	GetMetadataStatus(fileSystemID int64) bool
	FileSystemHasChild(fileSystemID int64) bool
	DeleteExport(exportID int64) (err error)
	DeleteExportRule(fileSystemID int64, ipAddress string) (err error)
	UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error)
	GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponce, error)
	RestoreFileSystemFromSnapShot(parentID, srcSnapShotID int64) (bool, error)

	*/

	GetFileSystemsByPool(poolID int, fsPrefix string) ([]FileSystem, error)

	/**
	GetFilesystemTreeqCount(fileSystemID int64) (treeqCnt int, err error)
	CreateTreeq(filesystemID int64, treeqParameter map[string]interface{}) (*Treeq, error)
	DeleteTreeq(fileSystemID, treeqID int64) (*Treeq, error)
	GetTreeq(fileSystemID, treeqID int64) (*Treeq, error)
	UpdateTreeq(fileSystemID, treeqID int64, body map[string]interface{}) (*Treeq, error)
	GetTreeqSizeByFileSystemID(filesystemID int64) (int64, error)
	GetFileSystemCountByPoolID(poolID int64) (int, error)
	GetMaxTreeqPerFs() (int, error)
	GetMaxFileSystems() (int, error)
	GetTreeqByName(fileSystemID int64, treeqName string) (*Treeq, error)
	*/

	// metadata
	PutMetadata(objectID int, metadata map[string]interface{}) (*PutMetadataResponse, error)
	GetMetadata(objectID int) ([]GetMetadataResult, error)
	DeleteMetadata(objectID int) (*DeleteMetadataResponse, error)

	// links
	GetLink(linkID int) (*Link, error)
	GetLinks() ([]Link, error)

	// system
	GetSystem() (*SystemDetails, error)
	/**
	// replication
	CreateReplica(request CreateReplicaRequest) (Replica, error)
	CreateCustomEvent(request CustomEventRequest) error
	*/
	CreateEvent(request EventRequest) error
}

type Credentials struct {
	Username string
	Password string
	Url      string
}

type IboxClient struct {
	Creds      Credentials
	Log        logr.Logger
	HttpClient *http.Client
}

func NewIboxClient(log logr.Logger, creds Credentials) (cl *IboxClient) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: tr}

	return &IboxClient{
		Creds:      creds,
		Log:        log,
		HttpClient: httpClient,
	}

}

func SetAuthHeader(req *http.Request, creds Credentials) {
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	// Set the Basic Auth header
	auth := creds.Username + ":" + creds.Password
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", basicAuth)
}
