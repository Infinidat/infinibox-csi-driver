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
package storage

import (
	"errors"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"math"
	"path"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Treeq count
	TREEQCOUNT = "host.k8s.treeqs"
)

// Operation declare for treeq count operation
type ACTION int

const (
	// Increment operation
	IncrementTreeqCount ACTION = 1 + iota
	// decrement operation
	DecrementTreeqCount
	NONE
)

// TreeqService file system services
type TreeqService struct {
	nfsstorage nfsstorage
	cs         Commonservice
	poolID     int64
	treeqCnt   int
}

type TreeqInterface interface {
	CreateTreeqVolume(storageClassParameters map[string]string, capacity int64, pVName string) (map[string]string, error)
	DeleteTreeqVolume(filesystemID, treeqID int64) error
	UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxFileSystemSize string) error
	IsTreeqAlreadyExist(poolName, networkSpace, pVName, fsPrefix string) (treeqVolume map[string]string, err error)
}

func (ts *TreeqService) checkTreeqName(FileSystems []api.FileSystem, pVName string) (treeqData *api.Treeq) {
	type item struct {
		treeq *api.Treeq
		err   error
	}
	itmArry := []item{}
	var wg sync.WaitGroup
	wg.Add(len(FileSystems))

	for _, f := range FileSystems {
		go func(f api.FileSystem) {
			var it item
			defer wg.Done()
			it.treeq, it.err = ts.cs.Api.GetTreeqByName(f.ID, pVName)
			itmArry = append(itmArry, it)
		}(f)
	}
	wg.Wait()
	for _, it := range itmArry {
		if it.err == nil && it.treeq != nil {
			treeqData = it.treeq
			return
		}
	}
	return
}

// IsTreeqAlreadyExist check the treeq exist or not
func (ts *TreeqService) IsTreeqAlreadyExist(poolName, networkSpace, pVName, fsPrefix string) (treeqVolumeContext map[string]string, err error) {
	zlog.Debug().Msgf("IsTreeqAlreadyExist called pool %s netspace %s pVName %s fsPrefix %s", poolName, networkSpace, pVName, fsPrefix)
	treeqVolumeContext = make(map[string]string)
	poolID, err := ts.cs.Api.GetStoragePoolIDByName(poolName)
	if err != nil {
		zlog.Error().Msgf("failed to get poolID from poolName %s", poolName)
		return
	}
	ts.poolID = poolID
	page := 1
	for {
		zlog.Debug().Msgf("IsTreeqAlreadyExist looking for file systems page %d", page)
		fsMetaData, poolErr := ts.cs.Api.GetFileSystemsByPoolID(poolID, page, fsPrefix)
		if poolErr != nil {
			zlog.Error().Msgf("failed to get filesystems from poolID %d and page no %d error %v", poolID, page, err)
			err = errors.New("failed to get filesystems from poolName " + poolName)
			return
		}
		if fsMetaData != nil && len(fsMetaData.FileSystemArry) == 0 {
			zlog.Debug().Msgf("IsTreeqAlreadyExist no file systems for this pool found")
			return
		}
		zlog.Debug().Msgf("IsTreeqAlreadyExist checking pv %s ", pVName)
		treeqData := ts.checkTreeqName(fsMetaData.FileSystemArry, pVName)
		if treeqData != nil {
			zlog.Debug().Msgf("treeq %s found to already exist", pVName)
			exportErr := ts.getExportPath(treeqData.FilesystemID) // fetch export path and set to filesystem exportPath
			if exportErr != nil {
				zlog.Error().Msgf("error getting export path %v", exportErr)
				err = exportErr
			}
			ipAddress, networkErr := ts.cs.getNetworkSpaceIP(networkSpace)
			if networkErr != nil {
				zlog.Error().Msgf("failed to get networkspace ipaddress %v", networkErr)
				err = exportErr
				return
			}
			ts.nfsstorage.ipAddress = ipAddress
			treeqVolumeContext["ID"] = strconv.FormatInt(treeqData.FilesystemID, 10)
			treeqVolumeContext["TREEQID"] = strconv.FormatInt(treeqData.ID, 10)
			treeqVolumeContext["ipAddress"] = ts.nfsstorage.ipAddress
			treeqVolumeContext["volumePath"] = path.Join(ts.nfsstorage.exportPath, treeqData.Path)
			zlog.Debug().Msgf("IsTreeqAlreadyExist copied treeqVolume %v", treeqVolumeContext)
			return
		}
		// inner for loop closed
		if fsMetaData.Filemetadata.PagesTotal == fsMetaData.Filemetadata.Page {
			zlog.Debug().Msgf("IsTreeqAlreadyExist no more pages")
			break
		}
		page++ // check the file system on next page
	} // outer for loop closed
	zlog.Debug().Msgf("IsTreeqAlreadyExist existing treeq not found")
	return
}

func (ts *TreeqService) getExpectedFileSystemID(maxFileSystemSize int64) (filesys *api.FileSystem, err error) {
	if ts.nfsstorage.capacity > maxFileSystemSize {
		zlog.Error().Msgf("not allowed to create treeq of size %d, max allowed size is %d", ts.nfsstorage.capacity, maxFileSystemSize)
		err = errors.New("request treeq size is greater than allowed max_filesystem_size")
		return
	}

	maxTreeqPerFS, err := ts.cs.Api.GetMaxTreeqPerFs()
	if err != nil {
		zlog.Error().Msgf("error getting ibox %s limit %s", common.SC_MAX_TREEQS_PER_FILESYSTEM, err.Error())
		return nil, err
	}

	// check for the storage class parameter is going to override
	v := ts.nfsstorage.storageClassParameters[common.SC_MAX_TREEQS_PER_FILESYSTEM]
	if v != "" {
		// use the storage class value
		maxTreeqPerFS, err = strconv.Atoi(v)
		if err != nil {
			zlog.Error().Msgf("error converting %s storage class parameter %s", common.SC_MAX_TREEQS_PER_FILESYSTEM, err.Error())
			return nil, err
		}
	}
	zlog.Debug().Msgf("%s limit being used %d\n", common.SC_MAX_TREEQS_PER_FILESYSTEM, maxTreeqPerFS)

	fsPrefix := ts.nfsstorage.storageClassParameters[common.SC_FS_PREFIX]
	if fsPrefix == "" {
		fsPrefix = common.SC_FS_PREFIX_DEFAULT
	}

	page := 1
	for {
		fsMetaData, poolErr := ts.cs.Api.GetFileSystemsByPoolID(ts.poolID, page, fsPrefix)
		if poolErr != nil {
			zlog.Error().Msgf("failed to get filesystems from poolID %d and page no %d error %v", ts.poolID, page, err)
			err = errors.New("failed to get filesystems from poolName " + ts.nfsstorage.storageClassParameters[common.SC_POOL_NAME])
			return
		}
		if fsMetaData != nil && len(fsMetaData.FileSystemArry) == 0 {
			zlog.Debug().Msgf("NO filesystem found.filesystem array is empty")
			return
		}
		for _, fs := range fsMetaData.FileSystemArry {
			if fs.Size+ts.nfsstorage.capacity < maxFileSystemSize {
				treeqCnt, treeqCnterr := ts.cs.Api.GetFilesystemTreeqCount(fs.ID)
				if treeqCnterr != nil {
					zlog.Error().Msgf("failed to get treeq count of filesystemID %d error %v", fs.ID, err)
					err = errors.New("failed to get treeq count of filesystemID " + strconv.FormatInt(fs.ID, 10))
					return
				}
				if treeqCnt < maxTreeqPerFS {
					ts.treeqCnt = treeqCnt
					zlog.Debug().Msgf("filesystem found to create treeQ,filesystemID %d", fs.ID)
					exportErr := ts.getExportPath(fs.ID) // fetch export path and set to filesystem exportPath
					if exportErr != nil {
						err = exportErr
					}
					filesys = &fs
					return
				}
			}
		} // inner for loop closed
		if fsMetaData.Filemetadata.PagesTotal == fsMetaData.Filemetadata.Page {
			break
		}
		page++ // check the file system on next page
	} // outer for loop closed
	zlog.Debug().Msgf("NO filesystem found to create treeQ")
	return
}

// CreateTreeqVolume create volume method
func (ts *TreeqService) CreateTreeqVolume(storageClassParameters map[string]string, capacity int64, pVName string) (treeqVolumeContext map[string]string, err error) {
	zlog.Debug().Msgf("CreateTreeqVolume filesystem.configmap %+v config %+v capacity %d pVName %s", ts.nfsstorage.storageClassParameters, storageClassParameters, capacity, pVName)

	treeqVolumeContext = map[string]string{}

	ts.nfsstorage.pVName = pVName
	ts.nfsstorage.storageClassParameters = storageClassParameters
	ts.nfsstorage.capacity = capacity
	ts.nfsstorage.exportPath = "/" + ts.nfsstorage.pVName

	ipAddress, err := ts.cs.getNetworkSpaceIP(strings.Trim(storageClassParameters[common.SC_NETWORK_SPACE], " "))
	if err != nil {
		zlog.Error().Msgf("failed to get networkspace ipaddress %v", err)
		return
	}
	ts.nfsstorage.ipAddress = ipAddress

	var poolID int64
	poolID, err = ts.cs.Api.GetStoragePoolIDByName(ts.nfsstorage.storageClassParameters[common.SC_POOL_NAME])
	if err != nil {
		zlog.Error().Msgf("failed to get poolID from poolName %s", ts.nfsstorage.storageClassParameters[common.SC_POOL_NAME])
		return
	}
	ts.poolID = poolID

	var maxFileSystemSize int64
	scMaxFileSystemSize := storageClassParameters[common.SC_MAX_FILESYSTEM_SIZE]
	if scMaxFileSystemSize == "" {
		// use the max int64 value which effively lets the ibox enforce any file system size limits
		maxFileSystemSize = math.MaxInt64
	} else {
		maxFileSystemSize, err = convertToByte(scMaxFileSystemSize)
		if err != nil {
			zlog.Error().Msgf("failed to convert storage class parameter %s value %s to byte", common.SC_MAX_FILESYSTEM_SIZE, scMaxFileSystemSize)
		}
	}

	var filesys *api.FileSystem
	helper.GetMutex().Mutex.Lock()
	defer helper.GetMutex().Mutex.Unlock()

	filesys, err = ts.getExpectedFileSystemID(maxFileSystemSize)
	if err != nil {
		zlog.Error().Msgf("failed to getExpectedFileSystemID  %v", err)
		return
	}
	var filesystemID int64
	if filesys == nil { // if pool is empty or no file system found to createTreeq
		pvSplit := strings.Split(ts.nfsstorage.pVName, "-")
		if len(pvSplit) < 2 {
			zlog.Error().Msgf("error with pvName format %+v, should have 2 parts", pvSplit)
			return
		}
		fsPrefix := ts.nfsstorage.storageClassParameters[common.SC_FS_PREFIX]
		if fsPrefix == "" {
			fsPrefix = common.SC_FS_PREFIX_DEFAULT
		}
		treeqFileSystemName := fsPrefix + pvSplit[1]

		ts.nfsstorage.exportPath = "/" + treeqFileSystemName
		err = ts.nfsstorage.createFileSystem(treeqFileSystemName)
		if err != nil {
			zlog.Error().Msgf("failed to create fileSystem %v", err)
			return
		}

		err = ts.nfsstorage.createExportPathAndAddMetadata()
		if err != nil {
			zlog.Error().Msgf("failed to create export and metadata %v", err)
			return
		}
		filesystemID = ts.nfsstorage.fileSystemID
	} else {
		filesystemID = filesys.ID
	}

	// create treeq
	treeqParameters := map[string]interface{}{
		"path":          path.Join("/", ts.nfsstorage.pVName),
		"name":          ts.nfsstorage.pVName,
		"hard_capacity": ts.nfsstorage.capacity,
	}
	treeqResponse, createTreeqerr := ts.cs.Api.CreateTreeq(filesystemID, treeqParameters)
	if createTreeqerr != nil {
		zlog.Error().Msgf("failed to create treeq  %s error %v", ts.nfsstorage.pVName, err)
		if filesys == nil { // if the file system created at the time of creating first treeq ,then delete the complete filesystem with export and metata
			deleteFilesystemErr := ts.cs.Api.DeleteFileSystemComplete(filesystemID)
			if deleteFilesystemErr != nil {
				zlog.Error().Msgf("failed to delete filesystem ,filesystemID = %d", filesystemID)
			}
		}
		err = errors.New("failed to create Treeq")
		return
	}

	treeqVolumeContext["ID"] = strconv.FormatInt(filesystemID, 10)
	treeqVolumeContext["TREEQID"] = strconv.FormatInt(treeqResponse.ID, 10)
	treeqVolumeContext["ipAddress"] = ts.nfsstorage.ipAddress
	treeqVolumeContext["volumePath"] = path.Join(ts.nfsstorage.exportPath, treeqResponse.Path)

	treeqCount := ts.treeqCnt + 1
	_, updateTreeqErr := ts.UpdateTreeqCnt(filesystemID, NONE, treeqCount)
	if updateTreeqErr != nil {
		err = errors.New("failed to increment treeq count as metadata")
		// if AttachMetadataToObject - failed to add metadata then delete the created treeq
		if ts.nfsstorage.fileSystemID != 0 {
			zlog.Debug().Msgf("error reverting treeq: %s", ts.nfsstorage.pVName)
			_, errDelTreeq := ts.cs.Api.DeleteTreeq(ts.nfsstorage.fileSystemID, treeqResponse.ID)
			if errDelTreeq != nil {
				zlog.Error().Msgf("failed to delete treeq: %s", ts.nfsstorage.pVName)
			}
		}
		return
	}

	// if new file system is created ,while creating the treeq, then not need to update size
	if filesys != nil {
		var updateFileSys api.FileSystem
		updateFileSys.Size = filesys.Size + ts.nfsstorage.capacity
		_, updateFileSizeErr := ts.cs.Api.UpdateFilesystem(filesystemID, updateFileSys)
		if updateFileSizeErr != nil {
			zlog.Error().Msgf("failed to update File Size %v", err)
			err = errors.New("failed to update files size")
			// if UpdateFilesystem fails, descrement the metadata tree count
			if filesystemID != 0 {
				zlog.Debug().Msgf("error reverting treeqcount")
				_, errUpdTreeq := ts.UpdateTreeqCnt(filesystemID, DecrementTreeqCount, 0)
				if errUpdTreeq != nil {
					zlog.Error().Msgf("failed to update count for treeq: %s", ts.nfsstorage.pVName)
				}
			}

			return
		}
	}
	return
}

func convertToByte(size string) (bytes int64, err error) {
	sizeUnits := map[string]int64{
		"gib": gib,
		"tib": tib,
	}
	for key, unit := range sizeUnits {
		if strings.Contains(size, strings.ToLower(key)) || strings.Contains(size, strings.ToUpper(key)) {
			arg := strings.Split(size, key)
			sizeUnit, errConvert := strconv.ParseInt(arg[0], 10, 64)
			if errConvert != nil {
				zlog.Error().Msgf("failed to convert the %s to bytes", size)
				return
			}
			bytes = sizeUnit * unit
			return
		}
	}
	err = errors.New("unexpected maxfilesystemsize, expected format: gib,tib")
	return
}

func (ts *TreeqService) getExportPath(filesystemID int64) error {
	exportResponse, exportErr := ts.cs.Api.GetExportByFileSystem(filesystemID)
	if exportErr != nil {
		zlog.Error().Msgf("failed to create export path of filesystem %d", filesystemID)
		return exportErr
	}
	for _, export := range *exportResponse {
		ts.nfsstorage.exportPath = export.ExportPath
		break
	}
	return nil
}

var deleteMutex sync.Mutex

// DeleteTreeqVolume delete volume method
func (ts *TreeqService) DeleteTreeqVolume(filesystemID, treeqID int64) (err error) {
	// 1. treeq exist or not checked
	var treeq *api.Treeq
	treeq, err = ts.cs.Api.GetTreeq(filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			err = errors.New("treeq does not exist on infinibox")
			return nil
		}
		zlog.Error().Msgf("Error occured while getting treeq: %s", err)
		return
	}

	// 2. if treeq has usedcapacity >0 then..
	if treeq.UsedCapacity > 0 {
		zlog.Error().Msgf("Can't delete NFS-treeq PV with data")
		err = errors.New("can't delete NFS-treeq PV with data")
		return
	}

	// 3. first decrement the treeq count to recover
	// In case of 1 - we are deleting the file system,
	deleteMutex.Lock()
	defer deleteMutex.Unlock()

	treeqCnt, err := ts.UpdateTreeqCnt(filesystemID, DecrementTreeqCount, 0)
	if err != nil {
		zlog.Error().Msgf("failed to update treeq count, filesystem: %s", ts.nfsstorage.pVName)
		return
	}
	// 4.delete the treeq
	_, err = ts.cs.Api.DeleteTreeq(filesystemID, treeqID)
	if err != nil {
		zlog.Error().Msgf("failed to delete treeq")
		if _, errUpdTreeq := ts.UpdateTreeqCnt(filesystemID, IncrementTreeqCount, 0); errUpdTreeq != nil {
			zlog.Error().Msgf("failed to update treeq count, filesystem: %s", ts.nfsstorage.pVName)
		}
		return
	}

	// 5.Delete file system if all treeq are delete
	if treeqCnt == 0 { // means all tree are delete. then delete the complete filesystem with exportPath ,metadata..etc
		err = ts.cs.Api.DeleteFileSystemComplete(filesystemID)
		if err != nil {
			zlog.Error().Msgf("failed to delete filesystem filesystemID %d error %v", filesystemID, err)
			return
		}
	}
	zlog.Debug().Msgf("Treeq deleted successfully")
	return
}

// UpdateTreeqCnt method
func (ts *TreeqService) UpdateTreeqCnt(fileSystemID int64, action ACTION, treeqCnt int) (treeqCount int, err error) {
	if treeqCnt == 0 {
		treeqCnt, err = ts.cs.Api.GetFilesystemTreeqCount(fileSystemID)
		if err != nil {
			return
		}
		zlog.Debug().Msgf("treeq count of fileSystemID: %d", fileSystemID)
	}

	switch action {
	case IncrementTreeqCount:
		treeqCnt++
	case DecrementTreeqCount:
		treeqCnt--
	}
	metadataParamter := map[string]interface{}{
		TREEQCOUNT: treeqCnt,
	}
	_, err = ts.cs.Api.AttachMetadataToObject(fileSystemID, metadataParamter)
	if err != nil {
		zlog.Error().Msgf("failed to update treeq count for filesystemID : %d error %v", fileSystemID, err)
		return
	}

	treeqCount = treeqCnt
	zlog.Debug().Msgf("treeq count updated successfully of fileSystemID: %d", fileSystemID)
	return
}

// UpdateTreeqVolume Update volume size method
func (svc *TreeqService) UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxFileSystemSize string) (err error) {

	// Get Filesystem
	fileSystemResponse, err := svc.cs.Api.GetFileSystemByID(filesystemID)
	if err != nil {
		zlog.Error().Msgf("failed to get file system %v", err)
		return
	}

	// Get a treeq
	treeq, err := svc.cs.Api.GetTreeq(filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			zlog.Debug().Msgf("treeq not found %d", treeqID)
			return nil
		}
		zlog.Error().Msgf("failed to get treeq: %s", err)
		return
	}

	// Get sum of all the treeq size of filesystem
	totalTreeqSize, err := svc.cs.Api.GetTreeqSizeByFileSystemID(filesystemID)
	if err != nil {
		zlog.Error().Msgf("failed to get sum of all the treeq sizes in a filesystem")
		return
	}

	needToIncreaseSize := capacity - treeq.HardCapacity
	if totalTreeqSize+needToIncreaseSize > fileSystemResponse.Size {
		var fileSys api.FileSystem
		freeSpace := fileSystemResponse.Size - totalTreeqSize
		increaseFileSizeBy := needToIncreaseSize - freeSpace
		fileSys.Size = fileSystemResponse.Size + increaseFileSizeBy

		// check to see if storage class has max file system size parameter set, if so, enforce the limit
		if maxFileSystemSize != "" {
			zlog.Debug().Msgf("performing max file system size limit check using storage class parameter %s", maxFileSystemSize)
			maxFileSystemSizeInBytes, err := convertToByte(maxFileSystemSize)
			if err != nil {
				zlog.Error().Msgf("failed to convert storage class parameter %s value %s to byte count", common.SC_MAX_FILESYSTEM_SIZE, maxFileSystemSize)
				return err
			}
			if fileSys.Size > maxFileSystemSizeInBytes {
				return status.Error(codes.PermissionDenied, "expansion capacity not allowed")
			}
		}

		// Expand file system size
		_, err = svc.cs.Api.UpdateFilesystem(filesystemID, fileSys)
		if err != nil {
			zlog.Error().Msgf("failed to update file system %v", err)
			return err
		}
	}

	// Expand Treeq size
	body := map[string]interface{}{"hard_capacity": capacity}
	_, err = svc.cs.Api.UpdateTreeq(filesystemID, treeqID, body)
	if err != nil {
		zlog.Error().Msgf("failed to update treeq size %v", err)
		return
	}

	zlog.Debug().Msg("treeq size updated successfully")
	return
}
