/*
Copyright 2022 Infinidat
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
package treeq

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/nfs"

	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Treeq count
	TreeqCount = "host.k8s.treeqs"
)

// Operation declare for treeq count operation
type Action int

const (
	// Increment operation
	IncrementTreeqCount Action = 1 + iota
	// decrement operation
	DecrementTreeqCount
	NONE
)

// Service file system services
type Service struct {
	NFSstorage nfs.NFSstorage
	CS         storagecommon.Commonservice
	PoolID     int
	TreeqCnt   int
}

type Interface interface {
	CreateTreeqVolume(ctx context.Context, StorageClassParameters map[string]string, capacity int64, pVName string) (map[string]string, error)
	DeleteTreeqVolume(ctx context.Context, filesystemID, treeqID int) error
	UpdateTreeqVolume(ctx context.Context, filesystemID, treeqID int, capacity int64, maxFileSystemSize string) error
	IsTreeqAlreadyExist(ctx context.Context, poolName, networkSpace, pVName, fsPrefix string) (treeqVolume map[string]string, err error)
}

// IsTreeqAlreadyExist check the treeq exist or not
func (ts *Service) IsTreeqAlreadyExist(ctx context.Context, poolName, networkSpace, persistentVolumeName, fileSystemPrefix string) (treeqVolumeContext map[string]string, err error) {
	slog.Debug("IsTreeqAlreadyExist", "pool", poolName, "ns", networkSpace, "pv name", persistentVolumeName, "fs prefix", fileSystemPrefix)
	treeqVolumeContext = make(map[string]string)
	pool, err := ts.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		slog.Error("failed to get poolID", "pool", poolName, "error", err.Error())
		return
	}
	ts.PoolID = pool.ID

	fileSystemMetaData, poolErr := ts.CS.IboxAPI.GetFileSystemsByPool(ctx, ts.PoolID, fileSystemPrefix)
	if poolErr != nil {
		slog.Error("failed to get filesystems", "pool id", ts.PoolID, "error", poolErr)
		err = errors.New("failed to get filesystems from poolName " + poolName)
		return
	}
	if fileSystemMetaData != nil && len(fileSystemMetaData) == 0 {
		slog.Debug("IsTreeqAlreadyExist no file systems for this pool found")
		return
	}
	slog.Debug("IsTreeqAlreadyExist checking pv", "pv", persistentVolumeName)
	treeqData := ts.checkTreeqName(ctx, fileSystemMetaData, persistentVolumeName)
	if treeqData != nil {
		slog.Debug("treeq found to already exist", "pvname", persistentVolumeName)
		exportErr := ts.getExportPath(ctx, treeqData.FilesystemID) // fetch export path and set to filesystem exportPath
		if exportErr != nil {
			slog.Error("error getting export path", "error", exportErr)
			err = exportErr
		}
		ipAddress, networkErr := ts.CS.GetNetworkSpaceIP(ctx, networkSpace)
		if networkErr != nil {
			slog.Error("failed to get networkspace ipaddress", "error", networkErr)
			err = exportErr
			return
		}
		ts.NFSstorage.IPAddress = ipAddress
		treeqVolumeContext["ID"] = strconv.Itoa(treeqData.FilesystemID)
		treeqVolumeContext["TREEQID"] = strconv.Itoa(treeqData.ID)
		treeqVolumeContext["ipAddress"] = ts.NFSstorage.IPAddress
		treeqVolumeContext["volumePath"] = path.Join(ts.NFSstorage.ExportPath, treeqData.Path)
		slog.Debug("IsTreeqAlreadyExist copied treeqVolume", "context", treeqVolumeContext)
		return
	}
	slog.Debug("IsTreeqAlreadyExist existing treeq not found")
	return
}

// CreateTreeqVolume create volume method
func (ts *Service) CreateTreeqVolume(ctx context.Context, storageClassParameters map[string]string, capacity int64, pVName string) (treeqVolumeContext map[string]string, err error) {
	slog.Debug("CreateTreeqVolume filesystem.configmap", "params", ts.NFSstorage.StorageClassParameters, "params2", storageClassParameters, "capacity", capacity, "pv name", pVName)

	treeqVolumeContext = map[string]string{}

	ts.NFSstorage.PVName = pVName
	ts.NFSstorage.StorageClassParameters = storageClassParameters
	ts.NFSstorage.Capacity = capacity
	ts.NFSstorage.ExportPath = "/" + ts.NFSstorage.PVName

	ipAddress, err := ts.CS.GetNetworkSpaceIP(ctx, strings.Trim(storageClassParameters[common.StorageClassNetworkSpace], " "))
	if err != nil {
		slog.Error("failed to get networkspace ipaddress", "error", err)
		return
	}
	ts.NFSstorage.IPAddress = ipAddress

	pool, err := ts.CS.IboxAPI.GetPoolByName(ctx, ts.NFSstorage.StorageClassParameters[common.StorageClassPoolName])
	if err != nil {
		slog.Error("failed to get poolID from poolName", "name", ts.NFSstorage.StorageClassParameters[common.StorageClassPoolName])
		return
	}
	ts.PoolID = pool.ID

	var maxFileSystemSize int64
	scMaxFileSystemSize := storageClassParameters[common.StorageClassMaxFilesystemSize]
	if scMaxFileSystemSize == "" {
		// use the max int64 value which effively lets the ibox enforce any file system size limits
		maxFileSystemSize = math.MaxInt64
	} else {
		maxFileSystemSize, err = convertToByte(scMaxFileSystemSize)
		if err != nil {
			slog.Error("failed to convert storage class parameter value to byte", "param", common.StorageClassMaxFilesystemSize, "param2", scMaxFileSystemSize)
		}
	}

	var filesys *iboxapi.FileSystem
	helper.GetMutex().Mutex.Lock()
	defer helper.GetMutex().Mutex.Unlock()

	filesys, err = ts.getExpectedFileSystemID(ctx, maxFileSystemSize)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("CreateTreeqVolume - getExpectedFilesystemID file system not found")
		} else {
			slog.Error("CreateTreeqVolume - error in getExpectedFileSystemID", "error", err)
			return
		}
	}
	var filesystemID int
	if filesys != nil {
		filesystemID = filesys.ID
	}

	if filesys == nil { // if pool is empty or no file system found to createTreeq
		pvSplit := strings.Split(ts.NFSstorage.PVName, "-")
		if len(pvSplit) < 2 {
			slog.Error("error with pvName format, should have 2 parts", "value", pvSplit)
			return
		}
		fsPrefix := ts.NFSstorage.StorageClassParameters[common.StorageClassFSPrefix]
		if fsPrefix == "" {
			fsPrefix = common.StorageClassFSPrefixDefault
		}
		treeqFileSystemName := fsPrefix + pvSplit[1]

		ts.NFSstorage.ExportPath = "/" + treeqFileSystemName
		err = ts.NFSstorage.CreateFileSystem(ctx, treeqFileSystemName)
		if err != nil {
			slog.Error("failed to create fileSystem", "error", err)
			return
		}

		err = ts.NFSstorage.CreateExportPathAndAddMetadata(ctx)
		if err != nil {
			slog.Error("failed to create export and metadata", "error", err)
			return
		}
		filesystemID = ts.NFSstorage.FileSystemID
	}

	// create treeq
	treeqParameters := iboxapi.CreateTreeqRequest{
		Path:         path.Join("/", ts.NFSstorage.PVName),
		Name:         ts.NFSstorage.PVName,
		HardCapacity: ts.NFSstorage.Capacity,
	}
	treeqResponse, createTreeqerr := ts.CS.IboxAPI.CreateTreeq(ctx, filesystemID, treeqParameters)
	if createTreeqerr != nil {
		slog.Error("failed to create treeq", "name", ts.NFSstorage.PVName, "error", err)
		if filesys == nil { // if the file system created at the time of creating first treeq ,then delete the complete filesystem with export and metata
			deleteFilesystemErr := ts.CS.API.DeleteFileSystemComplete(ctx, filesystemID)
			if deleteFilesystemErr != nil {
				slog.Error("failed to delete filesystem", "filesystemID", filesystemID)
			}
		}
		err = errors.New("failed to create Treeq")
		return
	}

	treeqVolumeContext["ID"] = strconv.Itoa(filesystemID)
	treeqVolumeContext["TREEQID"] = strconv.Itoa(treeqResponse.ID)
	treeqVolumeContext["ipAddress"] = ts.NFSstorage.IPAddress
	treeqVolumeContext["volumePath"] = path.Join(ts.NFSstorage.ExportPath, treeqResponse.Path)

	treeqCount := ts.TreeqCnt + 1
	_, updateTreeqErr := ts.UpdateTreeqCnt(ctx, filesystemID, NONE, treeqCount)
	if updateTreeqErr != nil {
		err = errors.New("failed to increment treeq count as metadata")
		// if AttachMetadataToObject - failed to add metadata then delete the created treeq
		if ts.NFSstorage.FileSystemID != 0 {
			slog.Debug("error reverting treeq", "pvname", ts.NFSstorage.PVName)
			_, errDelTreeq := ts.CS.IboxAPI.DeleteTreeq(ctx, ts.NFSstorage.FileSystemID, treeqResponse.ID)
			if errDelTreeq != nil {
				slog.Error("failed to delete tree", "pvname", ts.NFSstorage.PVName)
			}
		}
		return
	}

	// if new file system is created ,while creating the treeq, then not need to update size
	if filesys != nil {
		var updateFileSys iboxapi.FileSystem
		updateFileSys.Size = filesys.Size + ts.NFSstorage.Capacity
		_, updateFileSizeErr := ts.CS.IboxAPI.UpdateFileSystem(ctx, filesystemID, updateFileSys)
		if updateFileSizeErr != nil {
			slog.Error("failed to update File Size", "error", err)
			err = errors.New("failed to update files size")
			// if UpdateFilesystem fails, descrement the metadata tree count
			if filesystemID != 0 {
				slog.Debug("error reverting treeqcount")
				_, errUpdTreeq := ts.UpdateTreeqCnt(ctx, filesystemID, DecrementTreeqCount, 0)
				if errUpdTreeq != nil {
					slog.Error("failed to update count for treeq", "name", ts.NFSstorage.PVName)
				}
			}

			return
		}
	}
	return
}

func convertToByte(size string) (bytes int64, err error) {
	sizeUnits := map[string]int64{
		"gib": storagecommon.GIB,
		"tib": storagecommon.TIB,
	}
	for key, unit := range sizeUnits {
		if strings.Contains(size, strings.ToLower(key)) || strings.Contains(size, strings.ToUpper(key)) {
			arg := strings.Split(size, key)
			sizeUnit, errConvert := strconv.ParseInt(arg[0], 10, 64)
			if errConvert != nil {
				slog.Error("failed to convert to bytes", "size", size)
				return bytes, errConvert
			}
			bytes = sizeUnit * unit
			return
		}
	}
	err = errors.New("unexpected maxfilesystemsize, expected format: gib,tib")
	return
}

var deleteMutex sync.Mutex

// DeleteTreeqVolume delete volume method
func (ts *Service) DeleteTreeqVolume(ctx context.Context, filesystemID, treeqID int) (err error) {
	// 1. treeq exist or not checked
	var treeq *iboxapi.Treeq
	treeq, err = ts.CS.IboxAPI.GetTreeq(ctx, filesystemID, treeqID)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			//err = errors.New("treeq does not exist on infinibox")
			return
		}
		slog.Error("Error occurred while getting treeq", "error", err)
		return
	}

	// 2. if treeq has usedcapacity >0 then..
	if treeq.UsedCapacity > 0 {
		err = errors.New("can't delete NFS-treeq PV with data")
		slog.Error(err.Error())
		return err
	}

	// 3. first decrement the treeq count to recover
	// In case of 1 - we are deleting the file system,
	deleteMutex.Lock()
	defer deleteMutex.Unlock()

	treeqCnt, err := ts.UpdateTreeqCnt(ctx, filesystemID, DecrementTreeqCount, 0)
	if err != nil {
		slog.Error("failed to update treeq count", "filesystem", ts.NFSstorage.PVName)
		return
	}
	// 4.delete the treeq
	_, err = ts.CS.IboxAPI.DeleteTreeq(ctx, filesystemID, treeqID)
	if err != nil {
		slog.Error("failed to delete treeq")
		if _, errUpdTreeq := ts.UpdateTreeqCnt(ctx, filesystemID, IncrementTreeqCount, 0); errUpdTreeq != nil {
			slog.Error("failed to update treeq count", "filesystem", ts.NFSstorage.PVName)
		}
		return
	}

	// 5.Delete file system if all treeq are delete
	if treeqCnt == 0 { // means all tree are delete. then delete the complete filesystem with exportPath ,metadata..etc
		err = ts.CS.API.DeleteFileSystemComplete(ctx, filesystemID)
		if err != nil {
			slog.Error("failed to delete filesystem", "filesystemID", filesystemID, "error", err)
			return
		}
	}
	slog.Debug("Treeq deleted successfully")
	return
}

// UpdateTreeqCnt method
func (ts *Service) UpdateTreeqCnt(ctx context.Context, fileSystemID int, action Action, treeqCnt int) (treeqCount int, err error) {
	if treeqCnt == 0 {
		treeqs, err := ts.CS.IboxAPI.GetTreeqsByFileSystem(ctx, fileSystemID)
		if err != nil {
			return 0, err
		}
		treeqCnt = len(treeqs)
		slog.Debug("treeq count", "fileSystemID", fileSystemID, "count", treeqCnt)
	}

	switch action {
	case IncrementTreeqCount:
		treeqCnt++
	case DecrementTreeqCount:
		treeqCnt--
	}
	metadata := map[string]interface{}{
		TreeqCount: treeqCnt,
	}
	_, err = ts.CS.IboxAPI.PutMetadata(ctx, fileSystemID, metadata)
	if err != nil {
		slog.Error("failed to update treeq count", "filesystemID", fileSystemID, "error", err)
		return 0, err
	}

	treeqCount = treeqCnt
	slog.Debug("treeq count updated successfully", "fileSystemID", fileSystemID)
	return treeqCount, nil
}

// UpdateTreeqVolume Update volume size method
func (ts *Service) UpdateTreeqVolume(ctx context.Context, filesystemID, treeqID int, capacity int64, maxFileSystemSize string) (err error) {
	// Get Filesystem
	fileSystemResponse, err := ts.CS.IboxAPI.GetFileSystemByID(ctx, filesystemID)
	if err != nil {
		slog.Error("failed to get file system", "error", err)
		return
	}

	// Get a treeq
	treeq, err := ts.CS.IboxAPI.GetTreeq(ctx, filesystemID, treeqID)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("treeq not found", "treeqID", treeqID)
			return nil
		}
		slog.Error("failed to get treeq", "error", err)
		return
	}

	// Get sum of all the treeq size of filesystem
	treeqsInFileSystem, err := ts.CS.IboxAPI.GetTreeqsByFileSystem(ctx, filesystemID)
	if err != nil {
		slog.Error("failed to get sum of all the treeq sizes in a filesystem", "error", err.Error())
		return
	}
	var totalTreeqSize int64
	for _, t := range treeqsInFileSystem {
		totalTreeqSize += t.HardCapacity
	}

	needToIncreaseSize := capacity - treeq.HardCapacity
	if totalTreeqSize+needToIncreaseSize > fileSystemResponse.Size {
		var fileSys iboxapi.FileSystem
		freeSpace := fileSystemResponse.Size - totalTreeqSize
		increaseFileSizeBy := needToIncreaseSize - freeSpace
		fileSys.Size = fileSystemResponse.Size + increaseFileSizeBy

		// check to see if storage class has max file system size parameter set, if so, enforce the limit
		if maxFileSystemSize != "" {
			slog.Debug("performing max file system size limit check using storage class parameter", "param", maxFileSystemSize)
			maxFileSystemSizeInBytes, err := convertToByte(maxFileSystemSize)
			if err != nil {
				slog.Error("failed to convert storage class parameter to byte count", "value", common.StorageClassMaxFilesystemSize, "size", maxFileSystemSize)
				return err
			}
			if fileSys.Size > maxFileSystemSizeInBytes {
				return status.Error(codes.PermissionDenied, "expansion capacity not allowed")
			}
		}

		// Expand file system size
		_, err = ts.CS.IboxAPI.UpdateFileSystem(ctx, filesystemID, fileSys)
		if err != nil {
			slog.Error("failed to update file system", "error", err)
			return err
		}
	}

	// Expand Treeq size
	body := iboxapi.UpdateTreeqRequest{
		HardCapacity: capacity,
	}
	_, err = ts.CS.IboxAPI.UpdateTreeq(ctx, filesystemID, treeqID, body)
	if err != nil {
		slog.Error("failed to update treeq size", "error", err)
		return
	}

	slog.Debug("treeq size updated successfully")
	return
}

func (ts *Service) getExportPath(ctx context.Context, filesystemID int) error {
	exportResponse, exportErr := ts.CS.IboxAPI.GetExportsByFileSystemID(ctx, filesystemID)
	if exportErr != nil {
		slog.Error("failed to create export path", "filesystem", filesystemID)
		return exportErr
	}
	for _, export := range exportResponse {
		ts.NFSstorage.ExportPath = export.ExportPath
		break
	}
	return nil
}

func (ts *Service) getExpectedFileSystemID(ctx context.Context, maxFileSystemSize int64) (filesys *iboxapi.FileSystem, err error) {
	if ts.NFSstorage.Capacity > maxFileSystemSize {
		slog.Error("not allowed to create treeq", "size", ts.NFSstorage.Capacity, "max allowed size", maxFileSystemSize)
		err = errors.New("request treeq size is greater than allowed max_filesystem_size")
		return nil, err
	}

	maxTreeqPerFS, err := ts.CS.IboxAPI.GetMaxTreeqPerFs(ctx)
	if err != nil {
		slog.Error("error getting ibox limit", "value", common.StorageClassMaxTreeqsPerFS, "error", err.Error())
		return nil, err
	}

	// check for the storage class parameter is going to override
	tmpValue := ts.NFSstorage.StorageClassParameters[common.StorageClassMaxTreeqsPerFS]
	if tmpValue != "" {
		// use the storage class value
		maxTreeqPerFS, err = strconv.Atoi(tmpValue)
		if err != nil {
			slog.Error("error converting storage class parameters", "param", common.StorageClassMaxTreeqsPerFS, "error", err.Error())
			return nil, err
		}
	}
	slog.Debug("limit being used", "param", common.StorageClassMaxTreeqsPerFS, "limit", maxTreeqPerFS)

	fileSystemPrefix := ts.NFSstorage.StorageClassParameters[common.StorageClassFSPrefix]
	if fileSystemPrefix == "" {
		fileSystemPrefix = common.StorageClassFSPrefixDefault
	}

	fileSystemMetaData, poolErr := ts.CS.IboxAPI.GetFileSystemsByPool(ctx, ts.PoolID, fileSystemPrefix)
	if poolErr != nil {
		slog.Error("failed to get filesystems from poolID", "pool id", ts.PoolID, "error", err)
		err = errors.New("failed to get filesystems from poolName " + ts.NFSstorage.StorageClassParameters[common.StorageClassPoolName])
		return nil, err
	}
	if fileSystemMetaData != nil && len(fileSystemMetaData) == 0 {
		slog.Debug("NO filesystem found.filesystem array is empty")
		return nil, iboxapi.ErrNotFound
	}

	for _, fileSystem := range fileSystemMetaData {
		if fileSystem.Size+ts.NFSstorage.Capacity < maxFileSystemSize {
			treeqs, treeqCnterr := ts.CS.IboxAPI.GetTreeqsByFileSystem(ctx, fileSystem.ID)
			if treeqCnterr != nil {
				slog.Error("failed to get treeq count", "filesystem id", fileSystem.ID, "error", err)
				err = errors.New("failed to get treeq count of filesystemID " + strconv.Itoa(fileSystem.ID))
				return nil, err
			}
			if len(treeqs) < maxTreeqPerFS {
				ts.TreeqCnt = len(treeqs)
				slog.Debug("filesystem found to create treeQ", "filesystemID", fileSystem.ID)
				exportErr := ts.getExportPath(ctx, fileSystem.ID) // fetch export path and set to filesystem exportPath
				if exportErr != nil {
					return nil, exportErr
				}
				filesys = &fileSystem
				return filesys, nil
			}
		}
	}
	e := fmt.Errorf("NO filesystem found to create treeQ")
	slog.Debug(e.Error())
	return nil, iboxapi.ErrNotFound
}
func (ts *Service) checkTreeqName(ctx context.Context, fileSystems []iboxapi.FileSystem, persistentVolumeName string) (treeqData *iboxapi.Treeq) {
	type treeqInfo struct {
		treeq *iboxapi.Treeq
		err   error
	}
	items := []treeqInfo{}
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(fileSystems))

	for _, fileSystem := range fileSystems {
		go func(fileSystem iboxapi.FileSystem) {
			var treeqStuff treeqInfo
			defer waitGroup.Done()
			treeqStuff.treeq, treeqStuff.err = ts.CS.IboxAPI.GetTreeqByName(ctx, fileSystem.ID, persistentVolumeName)
			if treeqStuff.err != nil {
				slog.Error("checkTreeqName", "error", treeqStuff.err.Error())
			} else {
				items = append(items, treeqStuff)
			}
		}(fileSystem)
	}
	waitGroup.Wait()
	for _, item := range items {
		if item.err == nil && item.treeq != nil {
			treeqData = item.treeq
			return
		}
	}
	return
}
