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
	zlog.Debug().Msgf("IsTreeqAlreadyExist called pool %s netspace %s pVName %s fsPrefix %s", poolName, networkSpace, persistentVolumeName, fileSystemPrefix)
	treeqVolumeContext = make(map[string]string)
	pool, err := ts.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		zlog.Error().Msgf("failed to get poolID from poolName %s, error %s", poolName, err.Error())
		return
	}
	ts.PoolID = pool.ID

	fileSystemMetaData, poolErr := ts.CS.IboxAPI.GetFileSystemsByPool(ctx, ts.PoolID, fileSystemPrefix)
	if poolErr != nil {
		zlog.Error().Msgf("failed to get filesystems from poolID %d and error %v", ts.PoolID, poolErr)
		err = errors.New("failed to get filesystems from poolName " + poolName)
		return
	}
	if fileSystemMetaData != nil && len(fileSystemMetaData) == 0 {
		zlog.Debug().Msgf("IsTreeqAlreadyExist no file systems for this pool found")
		return
	}
	zlog.Debug().Msgf("IsTreeqAlreadyExist checking pv %s ", persistentVolumeName)
	treeqData := ts.checkTreeqName(ctx, fileSystemMetaData, persistentVolumeName)
	if treeqData != nil {
		zlog.Debug().Msgf("treeq %s found to already exist", persistentVolumeName)
		exportErr := ts.getExportPath(ctx, treeqData.FilesystemID) // fetch export path and set to filesystem exportPath
		if exportErr != nil {
			zlog.Error().Msgf("error getting export path %v", exportErr)
			err = exportErr
		}
		ipAddress, networkErr := ts.CS.GetNetworkSpaceIP(ctx, networkSpace)
		if networkErr != nil {
			zlog.Error().Msgf("failed to get networkspace ipaddress %v", networkErr)
			err = exportErr
			return
		}
		ts.NFSstorage.IPAddress = ipAddress
		treeqVolumeContext["ID"] = strconv.Itoa(treeqData.FilesystemID)
		treeqVolumeContext["TREEQID"] = strconv.Itoa(treeqData.ID)
		treeqVolumeContext["ipAddress"] = ts.NFSstorage.IPAddress
		treeqVolumeContext["volumePath"] = path.Join(ts.NFSstorage.ExportPath, treeqData.Path)
		zlog.Debug().Msgf("IsTreeqAlreadyExist copied treeqVolume %v", treeqVolumeContext)
		return
	}
	zlog.Debug().Msgf("IsTreeqAlreadyExist existing treeq not found")
	return
}

// CreateTreeqVolume create volume method
func (ts *Service) CreateTreeqVolume(ctx context.Context, storageClassParameters map[string]string, capacity int64, pVName string) (treeqVolumeContext map[string]string, err error) {
	zlog.Debug().Msgf("CreateTreeqVolume filesystem.configmap %+v config %+v capacity %d pVName %s", ts.NFSstorage.StorageClassParameters, storageClassParameters, capacity, pVName)

	treeqVolumeContext = map[string]string{}

	ts.NFSstorage.PVName = pVName
	ts.NFSstorage.StorageClassParameters = storageClassParameters
	ts.NFSstorage.Capacity = capacity
	ts.NFSstorage.ExportPath = "/" + ts.NFSstorage.PVName

	ipAddress, err := ts.CS.GetNetworkSpaceIP(ctx, strings.Trim(storageClassParameters[common.StorageClassNetworkSpace], " "))
	if err != nil {
		zlog.Error().Msgf("failed to get networkspace ipaddress %v", err)
		return
	}
	ts.NFSstorage.IPAddress = ipAddress

	pool, err := ts.CS.IboxAPI.GetPoolByName(ctx, ts.NFSstorage.StorageClassParameters[common.StorageClassPoolName])
	if err != nil {
		zlog.Error().Msgf("failed to get poolID from poolName %s", ts.NFSstorage.StorageClassParameters[common.StorageClassPoolName])
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
			zlog.Error().Msgf("failed to convert storage class parameter %s value %s to byte", common.StorageClassMaxFilesystemSize, scMaxFileSystemSize)
		}
	}

	var filesys *iboxapi.FileSystem
	helper.GetMutex().Mutex.Lock()
	defer helper.GetMutex().Mutex.Unlock()

	filesys, err = ts.getExpectedFileSystemID(ctx, maxFileSystemSize)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			zlog.Debug().Msgf("CreateTreeqVolume - getExpectedFilesystemID file system not found")
		} else {
			zlog.Error().Msgf("CreateTreeqVolume - error in getExpectedFileSystemID  %v", err)
			return
		}
	}
	var filesystemID int
	if filesys == nil { // if pool is empty or no file system found to createTreeq
		pvSplit := strings.Split(ts.NFSstorage.PVName, "-")
		if len(pvSplit) < 2 {
			zlog.Error().Msgf("error with pvName format %+v, should have 2 parts", pvSplit)
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
			zlog.Error().Msgf("failed to create fileSystem %v", err)
			return
		}

		err = ts.NFSstorage.CreateExportPathAndAddMetadata(ctx)
		if err != nil {
			zlog.Error().Msgf("failed to create export and metadata %v", err)
			return
		}
		filesystemID = ts.NFSstorage.FileSystemID
	} else {
		filesystemID = filesys.ID
	}

	// create treeq
	treeqParameters := iboxapi.CreateTreeqRequest{
		Path:         path.Join("/", ts.NFSstorage.PVName),
		Name:         ts.NFSstorage.PVName,
		HardCapacity: ts.NFSstorage.Capacity,
	}
	treeqResponse, createTreeqerr := ts.CS.IboxAPI.CreateTreeq(ctx, filesystemID, treeqParameters)
	if createTreeqerr != nil {
		zlog.Error().Msgf("failed to create treeq  %s error %v", ts.NFSstorage.PVName, err)
		if filesys == nil { // if the file system created at the time of creating first treeq ,then delete the complete filesystem with export and metata
			deleteFilesystemErr := ts.CS.API.DeleteFileSystemComplete(ctx, filesystemID)
			if deleteFilesystemErr != nil {
				zlog.Error().Msgf("failed to delete filesystem ,filesystemID = %d", filesystemID)
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
			zlog.Debug().Msgf("error reverting treeq: %s", ts.NFSstorage.PVName)
			_, errDelTreeq := ts.CS.IboxAPI.DeleteTreeq(ctx, ts.NFSstorage.FileSystemID, treeqResponse.ID)
			if errDelTreeq != nil {
				zlog.Error().Msgf("failed to delete treeq: %s", ts.NFSstorage.PVName)
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
			zlog.Error().Msgf("failed to update File Size %v", err)
			err = errors.New("failed to update files size")
			// if UpdateFilesystem fails, descrement the metadata tree count
			if filesystemID != 0 {
				zlog.Debug().Msgf("error reverting treeqcount")
				_, errUpdTreeq := ts.UpdateTreeqCnt(ctx, filesystemID, DecrementTreeqCount, 0)
				if errUpdTreeq != nil {
					zlog.Error().Msgf("failed to update count for treeq: %s", ts.NFSstorage.PVName)
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
				zlog.Error().Msgf("failed to convert the %s to bytes", size)
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
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			//err = errors.New("treeq does not exist on infinibox")
			return
		}
		zlog.Error().Msgf("Error occurred while getting treeq: %s", err)
		return
	}

	// 2. if treeq has usedcapacity >0 then..
	if treeq.UsedCapacity > 0 {
		err = errors.New("can't delete NFS-treeq PV with data")
		zlog.Error().Msg(err.Error())
		return err
	}

	// 3. first decrement the treeq count to recover
	// In case of 1 - we are deleting the file system,
	deleteMutex.Lock()
	defer deleteMutex.Unlock()

	treeqCnt, err := ts.UpdateTreeqCnt(ctx, filesystemID, DecrementTreeqCount, 0)
	if err != nil {
		zlog.Error().Msgf("failed to update treeq count, filesystem: %s", ts.NFSstorage.PVName)
		return
	}
	// 4.delete the treeq
	_, err = ts.CS.IboxAPI.DeleteTreeq(ctx, filesystemID, treeqID)
	if err != nil {
		zlog.Error().Msgf("failed to delete treeq")
		if _, errUpdTreeq := ts.UpdateTreeqCnt(ctx, filesystemID, IncrementTreeqCount, 0); errUpdTreeq != nil {
			zlog.Error().Msgf("failed to update treeq count, filesystem: %s", ts.NFSstorage.PVName)
		}
		return
	}

	// 5.Delete file system if all treeq are delete
	if treeqCnt == 0 { // means all tree are delete. then delete the complete filesystem with exportPath ,metadata..etc
		err = ts.CS.API.DeleteFileSystemComplete(ctx, filesystemID)
		if err != nil {
			zlog.Error().Msgf("failed to delete filesystem filesystemID %d error %v", filesystemID, err)
			return
		}
	}
	zlog.Debug().Msgf("Treeq deleted successfully")
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
		zlog.Debug().Msgf("treeq count of fileSystemID: %d is %d", fileSystemID, treeqCnt)
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
		zlog.Error().Msgf("failed to update treeq count for filesystemID : %d error %v", fileSystemID, err)
		return 0, err
	}

	treeqCount = treeqCnt
	zlog.Debug().Msgf("treeq count updated successfully of fileSystemID: %d", fileSystemID)
	return treeqCount, nil
}

// UpdateTreeqVolume Update volume size method
func (ts *Service) UpdateTreeqVolume(ctx context.Context, filesystemID, treeqID int, capacity int64, maxFileSystemSize string) (err error) {
	// Get Filesystem
	fileSystemResponse, err := ts.CS.IboxAPI.GetFileSystemByID(ctx, filesystemID)
	if err != nil {
		zlog.Error().Msgf("failed to get file system %v", err)
		return
	}

	// Get a treeq
	treeq, err := ts.CS.IboxAPI.GetTreeq(ctx, filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			zlog.Debug().Msgf("treeq not found %d", treeqID)
			return nil
		}
		zlog.Error().Msgf("failed to get treeq: %s", err)
		return
	}

	// Get sum of all the treeq size of filesystem
	treeqsInFileSystem, err := ts.CS.IboxAPI.GetTreeqsByFileSystem(ctx, filesystemID)
	if err != nil {
		zlog.Error().Msgf("failed to get sum of all the treeq sizes in a filesystem, %s", err.Error())
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
			zlog.Debug().Msgf("performing max file system size limit check using storage class parameter %s", maxFileSystemSize)
			maxFileSystemSizeInBytes, err := convertToByte(maxFileSystemSize)
			if err != nil {
				zlog.Error().Msgf("failed to convert storage class parameter %s value %s to byte count", common.StorageClassMaxFilesystemSize, maxFileSystemSize)
				return err
			}
			if fileSys.Size > maxFileSystemSizeInBytes {
				return status.Error(codes.PermissionDenied, "expansion capacity not allowed")
			}
		}

		// Expand file system size
		_, err = ts.CS.IboxAPI.UpdateFileSystem(ctx, filesystemID, fileSys)
		if err != nil {
			zlog.Error().Msgf("failed to update file system %v", err)
			return err
		}
	}

	// Expand Treeq size
	body := iboxapi.UpdateTreeqRequest{
		HardCapacity: capacity,
	}
	_, err = ts.CS.IboxAPI.UpdateTreeq(ctx, filesystemID, treeqID, body)
	if err != nil {
		zlog.Error().Msgf("failed to update treeq size %v", err)
		return
	}

	zlog.Debug().Msg("treeq size updated successfully")
	return
}

func (ts *Service) getExportPath(ctx context.Context, filesystemID int) error {
	exportResponse, exportErr := ts.CS.IboxAPI.GetExportsByFileSystemID(ctx, filesystemID)
	if exportErr != nil {
		zlog.Error().Msgf("failed to create export path of filesystem %d", filesystemID)
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
		zlog.Error().Msgf("not allowed to create treeq of size %d, max allowed size is %d", ts.NFSstorage.Capacity, maxFileSystemSize)
		err = errors.New("request treeq size is greater than allowed max_filesystem_size")
		return nil, err
	}

	maxTreeqPerFS, err := ts.CS.IboxAPI.GetMaxTreeqPerFs(ctx)
	if err != nil {
		zlog.Error().Msgf("error getting ibox %s limit %s", common.StorageClassMaxTreeqsPerFS, err.Error())
		return nil, err
	}

	// check for the storage class parameter is going to override
	tmpValue := ts.NFSstorage.StorageClassParameters[common.StorageClassMaxTreeqsPerFS]
	if tmpValue != "" {
		// use the storage class value
		maxTreeqPerFS, err = strconv.Atoi(tmpValue)
		if err != nil {
			zlog.Error().Msgf("error converting %s storage class parameter %s", common.StorageClassMaxTreeqsPerFS, err.Error())
			return nil, err
		}
	}
	zlog.Debug().Msgf("%s limit being used %d\n", common.StorageClassMaxTreeqsPerFS, maxTreeqPerFS)

	fileSystemPrefix := ts.NFSstorage.StorageClassParameters[common.StorageClassFSPrefix]
	if fileSystemPrefix == "" {
		fileSystemPrefix = common.StorageClassFSPrefixDefault
	}

	fileSystemMetaData, poolErr := ts.CS.IboxAPI.GetFileSystemsByPool(ctx, ts.PoolID, fileSystemPrefix)
	if poolErr != nil {
		zlog.Error().Msgf("failed to get filesystems from poolID %d and error %v", ts.PoolID, err)
		err = errors.New("failed to get filesystems from poolName " + ts.NFSstorage.StorageClassParameters[common.StorageClassPoolName])
		return nil, err
	}
	if fileSystemMetaData != nil && len(fileSystemMetaData) == 0 {
		zlog.Debug().Msgf("NO filesystem found.filesystem array is empty")
		return nil, &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("no filesystem found, array is empty")}
	}

	for _, fileSystem := range fileSystemMetaData {
		if fileSystem.Size+ts.NFSstorage.Capacity < maxFileSystemSize {
			treeqs, treeqCnterr := ts.CS.IboxAPI.GetTreeqsByFileSystem(ctx, fileSystem.ID)
			if treeqCnterr != nil {
				zlog.Error().Msgf("failed to get treeq count of filesystemID %d error %v", fileSystem.ID, err)
				err = errors.New("failed to get treeq count of filesystemID " + strconv.Itoa(fileSystem.ID))
				return nil, err
			}
			if len(treeqs) < maxTreeqPerFS {
				ts.TreeqCnt = len(treeqs)
				zlog.Debug().Msgf("filesystem found to create treeQ,filesystemID %d", fileSystem.ID)
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
	zlog.Debug().Msg(e.Error())
	return nil, &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: e}
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
				zlog.Error().Msgf("checkTreeqName error %s", treeqStuff.err.Error())
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
