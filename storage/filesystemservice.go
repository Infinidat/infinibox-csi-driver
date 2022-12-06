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
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	"path"
	"strconv"
	"strings"
	"sync"

	"k8s.io/klog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// treeq constants
const (
	PROVISIONTYPE          = "provision_type"
	MAXTREEQSPERFILESYSTEM = "max_treeqs_per_filesystem"
	MAXFILESYSTEMS         = "max_filesystems"
	MAXFILESYSTEMSIZE      = "max_filesystem_size"
	//	UNIXPERMISSION         = "nfs_unix_permissions"
	FSPREFIX = "fs_prefix"

	// Treeq count
	TREEQCOUNT = "host.k8s.treeqs"
)

// service type
const (
	NFSTREEQ             = "nfs_treeq"
	NFS                  = "nfs"
	TreeqUnixPermissions = "750"
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

// FilesystemService file system services
type FilesystemService struct {
	configmap map[string]string // values from storage class
	pVName    string
	capacity  int64

	fileSystemID int64
	exportpath   string
	exportID     int64
	exportBlock  string
	ipAddress    string

	cs       commonservice
	poolID   int64
	treeqCnt int

	treeqVolume map[string]string
}

func getFilesystemService(serviceType string, c commonservice) *FilesystemService {
	if NFSTREEQ == serviceType {
		return &FilesystemService{
			cs:          c,
			treeqVolume: make(map[string]string),
		}
	}
	return nil
}

// FileSystemInterface interface
type FileSystemInterface interface {
	CreateTreeqVolume(config map[string]string, capacity int64, pvName string) (map[string]string, error)
	DeleteTreeqVolume(filesystemID, treeqID int64) error
	UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxSize string) error
	IsTreeqAlreadyExist(pool_name, network_space, pVName string) (treeqVolume map[string]string, err error)
}

func (filesystem *FilesystemService) checkTreeqName(FileSystemArry []api.FileSystem, pVName string) (treeqData *api.Treeq) {
	type item struct {
		treeq *api.Treeq
		err   error
	}
	itmArry := []item{}
	var wg sync.WaitGroup
	wg.Add(len(FileSystemArry))

	for _, f := range FileSystemArry {
		go func(f api.FileSystem) {
			var it item
			defer wg.Done()
			it.treeq, it.err = filesystem.cs.api.GetTreeqByName(f.ID, pVName)
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
func (filesystem *FilesystemService) IsTreeqAlreadyExist(pool_name, network_space, pVName string) (treeqVolume map[string]string, err error) {
	klog.V(4).Infof("IsTreeqAlreadyExist called pool %s netspace %s pVName %s", pool_name, network_space, pVName)
	treeqVolume = make(map[string]string)
	poolID, err := filesystem.cs.api.GetStoragePoolIDByName(pool_name)
	if err != nil {
		klog.Errorf("failed to get poolID from poolName %s", pool_name)
		return
	}
	filesystem.poolID = poolID
	page := 1
	for {
		klog.V(4).Infof("IsTreeqAlreadyExist looking for file systems page %d", page)
		fsMetaData, poolErr := filesystem.cs.api.GetFileSystemsByPoolID(poolID, page)
		if poolErr != nil {
			klog.Errorf("failed to get filesystems from poolID %d and page no %d error %v", poolID, page, err)
			err = errors.New("failed to get filesystems from poolName " + pool_name)
			return
		}
		if fsMetaData != nil && len(fsMetaData.FileSystemArry) == 0 {
			klog.V(4).Infof("IsTreeqAlreadyExist no file systems for this pool found")
			return
		}
		klog.V(4).Infof("IsTreeqAlreadyExist checking pv %s ", pVName)
		treeqData := filesystem.checkTreeqName(fsMetaData.FileSystemArry, pVName)
		if treeqData != nil {
			klog.V(4).Infof("treeq %s found to already exist", pVName)
			exportErr := filesystem.getExportPath(treeqData.FilesystemID) // fetch export path and set to filesystem exportPath
			if exportErr != nil {
				klog.Errorf("error getting export path %v", exportErr)
				err = exportErr
			}
			ipAddress, networkErr := filesystem.cs.getNetworkSpaceIP(network_space)
			if networkErr != nil {
				klog.Errorf("failed to get networkspace ipaddress %v", networkErr)
				err = exportErr
				return
			}
			filesystem.ipAddress = ipAddress
			treeqVolume["ID"] = strconv.FormatInt(treeqData.FilesystemID, 10)
			treeqVolume["TREEQID"] = strconv.FormatInt(treeqData.ID, 10)
			treeqVolume["ipAddress"] = filesystem.ipAddress
			treeqVolume["volumePath"] = path.Join(filesystem.exportpath, treeqData.Path)
			klog.V(4).Infof("IsTreeqAlreadyExist copied treeqVolume %v", treeqVolume)
			return
		}
		// inner for loop closed
		if fsMetaData.Filemetadata.PagesTotal == fsMetaData.Filemetadata.Page {
			klog.V(4).Infof("IsTreeqAlreadyExist no more pages")
			break
		}
		page++ // check the file system on next page
	} // outer for loop closed
	klog.V(4).Infof("IsTreeqAlreadyExist existing treeq not found")
	return
}

func (filesystem *FilesystemService) getExpectedFileSystemID(maxFileSystemSize int64) (filesys *api.FileSystem, err error) {
	if filesystem.capacity > maxFileSystemSize {
		klog.Errorf("Can't allowed to create treeq of size %d", filesystem.capacity)
		klog.Errorf("Max allowed filesytem size %d", maxFileSystemSize)
		err = errors.New("Request treeq size is greater than allowed max_filesystem_size")
		return
	}
	page := 1
	for {
		fsMetaData, poolErr := filesystem.cs.api.GetFileSystemsByPoolID(filesystem.poolID, page)
		if poolErr != nil {
			klog.Errorf("failed to get filesystems from poolID %d and page no %d error %v", filesystem.poolID, page, err)
			err = errors.New("failed to get filesystems from poolName " + filesystem.configmap["pool_name"])
			return
		}
		if fsMetaData != nil && len(fsMetaData.FileSystemArry) == 0 {
			klog.V(4).Infof("NO filesystem found.filesystem array is empty")
			return
		}
		for _, fs := range fsMetaData.FileSystemArry {
			if fs.Size+filesystem.capacity < maxFileSystemSize {
				treeqCnt, treeqCnterr := filesystem.cs.api.GetFilesytemTreeqCount(fs.ID)
				if treeqCnterr != nil {
					klog.Errorf("failed to get treeq count of filesystemID %d error %v", fs.ID, err)
					err = errors.New("failed to get treeq count of filesystemID " + strconv.FormatInt(fs.ID, 10))
					return
				}
				if treeqCnt < filesystem.getAllowedCount(MAXTREEQSPERFILESYSTEM) {
					filesystem.treeqCnt = treeqCnt
					klog.V(4).Infof("filesystem found to create treeQ,filesystemID %d", fs.ID)
					exportErr := filesystem.getExportPath(fs.ID) // fetch export path and set to filesystem exportPath
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
	klog.V(4).Infof("NO filesystem found to create treeQ")
	return
}

func (filesystem *FilesystemService) setParameter(config map[string]string, capacity int64, pvName string) {
	filesystem.pVName = pvName
	filesystem.configmap = config
	filesystem.capacity = capacity
	filesystem.exportpath = "/" + filesystem.pVName
}

// CreateTreeqVolume create volume method
func (filesystem *FilesystemService) CreateTreeqVolume(config map[string]string, capacity int64, pvName string) (treeqVolume map[string]string, err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while creating treeq method " + fmt.Sprint(res))
		}
	}()
	treeqVolume = make(map[string]string)
	treeqVolume["storage_protocol"] = config["storage_protocol"]
	treeqVolume["gid"] = config["gid"]
	treeqVolume["uid"] = config["uid"]
	treeqVolume["unix_permissions"] = config["unix_permissions"]
	filesystem.setParameter(config, capacity, pvName)

	ipAddress, err := filesystem.cs.getNetworkSpaceIP(strings.Trim(config["network_space"], " "))
	if err != nil {
		klog.Errorf("failed to get networkspace ipaddress %v", err)
		return
	}
	filesystem.ipAddress = ipAddress

	var poolID int64
	poolID, err = filesystem.cs.api.GetStoragePoolIDByName(filesystem.configmap["pool_name"])
	if err != nil {
		klog.Errorf("failed to get poolID from poolName %s", filesystem.configmap["pool_name"])
		return
	}
	filesystem.poolID = poolID

	maxFileSystemSize, err := filesystem.maxFileSize()
	if err != nil {
		klog.Errorf(err.Error())
		return
	}

	var filesys *api.FileSystem
	helper.GetMutex().Mutex.Lock()
	defer helper.GetMutex().Mutex.Unlock()

	filesys, err = filesystem.getExpectedFileSystemID(maxFileSystemSize)
	if err != nil {
		klog.Errorf("failed to getExpectedFileSystemID  %v", err)
		return
	}
	var filesystemID int64
	if filesys == nil { // if pool is empty or no file system found to createTreeq
		err = filesystem.createFileSystem()
		if err != nil {
			klog.Errorf("failed to create fileSystem %v", err)
			return
		}
		err = filesystem.createExportPathAndAddMetadata()
		if err != nil {
			klog.Errorf("failed to create export and metadata %v", err)
			return
		}
		filesystemID = filesystem.fileSystemID
	} else {
		filesystemID = filesys.ID
	}

	// create treeq
	treeqResponse, createTreeqerr := filesystem.cs.api.CreateTreeq(filesystemID, filesystem.getTreeParameters())
	if createTreeqerr != nil {
		klog.Errorf("failed to create treeq  %s error %v", filesystem.pVName, err)
		if filesys == nil { // if the file system created at the time of creating first treeq ,then delete the complete filesystem with export and metata
			deleteFilesystemErr := filesystem.cs.api.DeleteFileSystemComplete(filesystemID)
			if deleteFilesystemErr != nil {
				klog.Errorf("failed to delete filesystem ,filesystemID = %d", filesystemID)
			}
		}
		err = errors.New("failed to create Treeq")
		return
	}
	treeqVolume["ID"] = strconv.FormatInt(filesystemID, 10)
	treeqVolume["TREEQID"] = strconv.FormatInt(treeqResponse.ID, 10)
	treeqVolume["ipAddress"] = filesystem.ipAddress
	treeqVolume["volumePath"] = path.Join(filesystem.exportpath, treeqResponse.Path)

	// if AttachMetadataToObject - failed to add metadata then delete the created treeq
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while update metadata" + fmt.Sprint(res))
		}
		if err != nil && filesystem.fileSystemID != 0 {
			klog.V(2).Infof("Seemes to be some problem reverting treeq: %s", filesystem.pVName)
			_, errDelTreeq := filesystem.cs.api.DeleteTreeq(filesystem.fileSystemID, treeqResponse.ID)
			if errDelTreeq != nil {
				klog.Errorf("failed to delete treeq: %s", filesystem.pVName)
			}
		}
	}()

	treeqCount := filesystem.treeqCnt + 1
	_, updateTreeqErr := filesystem.UpdateTreeqCnt(filesystemID, NONE, treeqCount)
	if updateTreeqErr != nil {
		err = errors.New("failed to increment treeq count as metadata")
		return
	}

	// if UpdateFilesystem fails, descrement the metadata tree count
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while update file size" + fmt.Sprint(res))
		}
		if err != nil && filesystemID != 0 {
			klog.V(2).Infof("Seemes to be some problem reverting treeqcount")
			_, errUpdTreeq := filesystem.UpdateTreeqCnt(filesystemID, DecrementTreeqCount, 0)
			if errUpdTreeq != nil {
				klog.Errorf("failed to update count for treeq: %s", filesystem.pVName)
			}
		}
	}()

	// if new file system is created ,while creating the treeq, then not need to update size
	if filesys != nil {
		var updateFileSys api.FileSystem
		updateFileSys.Size = filesys.Size + filesystem.capacity
		_, updateFileSizeErr := filesystem.cs.api.UpdateFilesystem(filesystemID, updateFileSys)
		if updateFileSizeErr != nil {
			klog.Errorf("failed to update File Size %v", err)
			err = errors.New("failed to update files size")
			return
		}
	}
	return
}

func (filesystem *FilesystemService) createExportPathAndAddMetadata() (err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while export directory" + fmt.Sprint(res))
		}
		if err != nil && filesystem.fileSystemID != 0 {
			klog.V(2).Infof("Seemes to be some problem reverting filesystem: %s", filesystem.pVName)
			if _, errDelFS := filesystem.cs.api.DeleteFileSystem(filesystem.fileSystemID); errDelFS != nil {
				klog.Errorf("failed to delete filesystem: %s", filesystem.pVName)
			}
		}
	}()

	err = filesystem.createExportPath()
	if err != nil {
		klog.Errorf("failed to export path %v", err)
		return
	}
	klog.V(4).Infof("export path created for filesystem: %s", filesystem.pVName)

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while AttachMetadata directory" + fmt.Sprint(res))
		}
		if err != nil && filesystem.exportID != 0 {
			klog.V(2).Info("Seemes to be some problem reverting created export id:", filesystem.exportID)
			if _, errDelExport := filesystem.cs.api.DeleteExportPath(filesystem.exportID); errDelExport != nil {
				klog.Errorf("failed to delete export path: %s", filesystem.pVName)
			}
		}
	}()
	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = filesystem.pVName
	metadata["host.created_by"] = filesystem.cs.GetCreatedBy()

	_, err = filesystem.cs.api.AttachMetadataToObject(filesystem.fileSystemID, metadata)
	if err != nil {
		klog.Errorf("failed to attach metadata for fileSystem : %s", filesystem.pVName)
		klog.Errorf("error to attach metadata %v", err)
		return
	}
	klog.V(4).Infof("metadata attached successfully for filesystem %s", filesystem.pVName)
	return
}

func (filesystem *FilesystemService) createFileSystem() (err error) {
	fileSystemCnt, err := filesystem.cs.api.GetFileSystemCountByPoolID(filesystem.poolID)
	if err != nil {
		klog.Errorf("failed to get the filesystem count from Ibox %v", err)
		return
	}
	if fileSystemCnt >= filesystem.getAllowedCount(MAXFILESYSTEMS) {
		klog.V(4).Infof("Max filesystem allowed on Pool %v", filesystem.getAllowedCount(MAXFILESYSTEMS))
		klog.V(4).Infof("Current filesystem count on Pool %v", fileSystemCnt)
		klog.Errorf("Ibox not allowed to create new file system")
		err = errors.New("Ibox not allowed to create new file system")
		return
	}
	ssdEnabled := filesystem.configmap["ssd_enabled"]
	if ssdEnabled == "" {
		ssdEnabled = fmt.Sprint(false)
	}
	ssd, _ := strconv.ParseBool(ssdEnabled)
	mapRequest := make(map[string]interface{})
	mapRequest["pool_id"] = filesystem.poolID

	var treeqFileSystemName string

	pvSplit := strings.Split(filesystem.pVName, "-")
	treeqFileSystemName = "csit_" + pvSplit[1]

	if prefix, ok := filesystem.configmap[FSPREFIX]; ok {
		treeqFileSystemName = prefix + pvSplit[1]
	}
	filesystem.exportpath = "/" + treeqFileSystemName
	mapRequest["name"] = treeqFileSystemName
	mapRequest["ssd_enabled"] = ssd
	mapRequest["provtype"] = strings.ToUpper(filesystem.configmap["provision_type"])
	mapRequest["size"] = filesystem.capacity
	fileSystem, err := filesystem.cs.api.CreateFilesystem(mapRequest)
	if err != nil {
		klog.Errorf("failed to create filesystem %s", filesystem.pVName)
		return
	}
	filesystem.fileSystemID = fileSystem.ID
	klog.V(4).Infof("filesystem Created %s", filesystem.pVName)
	return
}

func (filesystem *FilesystemService) createExportPath() (err error) {
	permissionsMapArray, err := getPermissionMaps(filesystem.configmap["nfs_export_permissions"])
	if err != nil {
		return
	}

	var exportFileSystem api.ExportFileSys
	exportFileSystem.FilesystemID = filesystem.fileSystemID
	exportFileSystem.Transport_protocols = "TCP"
	exportFileSystem.Privileged_port = false
	exportFileSystem.Export_path = filesystem.exportpath
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	exportResp, err := filesystem.cs.api.ExportFileSystem(exportFileSystem)
	if err != nil {
		klog.Errorf("failed to create export path of filesystem %s", filesystem.pVName)
		return
	}
	filesystem.exportID = exportResp.ID
	filesystem.exportBlock = exportResp.ExportPath
	return
}

func getDefaultValues() map[string]string {
	defaultConfigMap := make(map[string]string)
	defaultConfigMap[PROVISIONTYPE] = "thin"
	defaultConfigMap[MAXTREEQSPERFILESYSTEM] = "1000"
	defaultConfigMap[MAXFILESYSTEMS] = "1000"
	defaultConfigMap[MAXFILESYSTEMSIZE] = "100tib"
	// defaultConfigMap[UNIXPERMISSION] = "750"
	return defaultConfigMap
}

func (filesystem *FilesystemService) getAllowedCount(key string) int {
	var allowedCnt int
	if _, ok := filesystem.configmap[key]; ok {
		allowedCnt, err := strconv.Atoi(filesystem.configmap[key])
		if err == nil {
			return allowedCnt
		}
	}
	defaultConfigMap := getDefaultValues()
	val := defaultConfigMap[key]
	allowedCnt, _ = strconv.Atoi(val)
	return allowedCnt
}

func (filesystem *FilesystemService) maxFileSize() (sizeInByte int64, err error) {
	if maxfilesize, ok := filesystem.configmap[MAXFILESYSTEMSIZE]; ok {
		sizeInByte, err = convertToByte(maxfilesize)
		if err != nil {
			klog.Errorf("failed to convert MAXFILESYSTEMSIZE %s to byte", maxfilesize)
		}
		return
	}
	defaultSize := getDefaultValues()[MAXFILESYSTEMSIZE]
	sizeInByte, err = convertToByte(defaultSize)
	return
}

func convertToByte(size string) (bytes int64, err error) {
	sizeUnits := make(map[string]int64)
	sizeUnits["gib"] = gib
	sizeUnits["tib"] = tib
	for key, unit := range sizeUnits {
		if strings.Contains(size, strings.ToLower(key)) || strings.Contains(size, strings.ToUpper(key)) {
			arg := strings.Split(size, key)
			sizeUnit, errConvert := strconv.ParseInt(arg[0], 10, 64)
			if errConvert != nil {
				klog.Errorf("failed to convert the %s to bytes", size)
				return
			}
			bytes = sizeUnit * unit
			return
		}
	}
	err = errors.New("unexpected maxfilesystemsize, expected format: gib,tib")
	return
}

func (filesystem *FilesystemService) getTreeParameters() map[string]interface{} {
	treeqParameter := make(map[string]interface{})
	treeqParameter["path"] = path.Join("/", filesystem.pVName)
	treeqParameter["name"] = filesystem.pVName
	treeqParameter["hard_capacity"] = filesystem.capacity
	return treeqParameter
}

// func (filesystem *FilesystemService) getTreeModePermission() string {
// 	if unixPermission, ok := filesystem.configmap[UNIXPERMISSION]; ok {
// 		return unixPermission
// 	}
// 	values := getDefaultValues()
// 	return values[UNIXPERMISSION]
// }

func (filesystem *FilesystemService) getExportPath(filesystemID int64) error {
	exportResponse, exportErr := filesystem.cs.api.GetExportByFileSystem(filesystemID)
	if exportErr != nil {
		klog.Errorf("failed to create export path of filesystem %d", filesystemID)
		return exportErr
	}
	for _, export := range *exportResponse {
		filesystem.exportpath = export.ExportPath
		break
	}
	return nil
}

func isTreeQEmpty(treeq api.Treeq) bool {
	return treeq.UsedCapacity <= 0
}

var deleteMutex sync.Mutex

// DeleteNFSVolume delete volume method
func (filesystem *FilesystemService) DeleteTreeqVolume(filesystemID, treeqID int64) (err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while deleting treeq " + fmt.Sprint(res))
			return
		}
	}()
	// 1.treeq exist or not checked
	var treeq *api.Treeq
	treeq, err = filesystem.cs.api.GetTreeq(filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			err = errors.New("treeq does not exist on infinibox")
			return nil
		}
		klog.Errorf("Error occured while getting treeq: %s", err)
		return
	}

	// 2.if treeq has usedcapacity >0 then..
	if !isTreeQEmpty(*treeq) {
		klog.Errorf("Can't delete NFS-treeq PV with data")
		err = errors.New("can't delete NFS-treeq PV with data")
		return
	}

	// 3 first decremnt the treeq count to recover
	// In case of 1 - we are deleting the file system,
	deleteMutex.Lock()
	defer deleteMutex.Unlock()

	treeqCnt, err := filesystem.UpdateTreeqCnt(filesystemID, DecrementTreeqCount, 0)
	if err != nil {
		klog.Errorf("failed to update treeq count, filesystem: %s", filesystem.pVName)
		return
	}
	// 4.delete the treeq
	_, err = filesystem.cs.api.DeleteTreeq(filesystemID, treeqID)
	if err != nil {
		klog.Errorf("failed to delete treeq")
		if _, errUpdTreeq := filesystem.UpdateTreeqCnt(filesystemID, IncrementTreeqCount, 0); errUpdTreeq != nil {
			klog.Errorf("failed to update treeq count, filesystem: %s", filesystem.pVName)
		}
		return
	}

	// 5.Delete file system if all treeq are delete
	if treeqCnt == 0 { // measn all tree are delete. then delete the complete filesystem with exportPath ,metadata..etc
		err = filesystem.cs.api.DeleteFileSystemComplete(filesystemID)
		if err != nil {
			klog.Errorf("failed to delete filesystem filesystemID %d error %v", filesystemID, err)
			return
		}
	}
	klog.V(4).Infof("Treeq deleted successfully")
	return
}

// UpdateTreeqCnt method
func (filesystem *FilesystemService) UpdateTreeqCnt(fileSystemID int64, action ACTION, treeqCnt int) (treeqCount int, err error) {
	if treeqCnt == 0 {
		treeqCnt, err = filesystem.cs.api.GetFilesytemTreeqCount(fileSystemID)
		if err != nil {
			return
		}
		klog.V(4).Infof("treeq count of fileSystemID: %d", fileSystemID)
	}

	switch action {
	case IncrementTreeqCount:
		treeqCnt++
	case DecrementTreeqCount:
		treeqCnt--
	}
	metadataParamter := make(map[string]interface{})
	metadataParamter[TREEQCOUNT] = treeqCnt
	_, err = filesystem.cs.api.AttachMetadataToObject(fileSystemID, metadataParamter)
	if err != nil {
		klog.Errorf("failed to update treeq count for filesystemID : %d error %v", fileSystemID, err)
		return
	}

	treeqCount = treeqCnt
	klog.V(4).Infof("treeq count updated successfully of fileSystemID: %d", fileSystemID)
	return
}

// UpdateTreeqVolume Upadate volume size method
func (filesystem *FilesystemService) UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxSize string) (err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("failed to update treeq " + fmt.Sprint(res))
			return
		}
	}()

	// Get Filesystem
	fileSystemResponse, err := filesystem.cs.api.GetFileSystemByID(filesystemID)
	if err != nil {
		klog.Errorf("failed to get file system %v", err)
		return
	}

	// Get a treeq
	treeq, err := filesystem.cs.api.GetTreeq(filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			err = errors.New("treeq not found")
			return nil
		}
		klog.Errorf("failed to get treeq: %s", err)
		return
	}

	// Get sum of all the treeq size of filesystem
	totalTreeqSize, err := filesystem.cs.api.GetTreeqSizeByFileSystemID(filesystemID)
	if err != nil {
		klog.Errorf("failed to get sum of all the treeq sizes in a filesystem")
		return
	}

	needToIncreaseSize := capacity - treeq.HardCapacity
	if totalTreeqSize+needToIncreaseSize > fileSystemResponse.Size {
		configMap := make(map[string]string)
		configMap[MAXFILESYSTEMSIZE] = maxSize
		filesystem.configmap = configMap
		// Get Maximum filesystem size
		maxFileSystemSize, err := filesystem.maxFileSize()
		if err != nil {
			klog.Errorf(err.Error())
			return err
		}

		var fileSys api.FileSystem
		freeSpace := fileSystemResponse.Size - totalTreeqSize
		increaseFileSizeBy := needToIncreaseSize - freeSpace
		fileSys.Size = fileSystemResponse.Size + increaseFileSizeBy
		if fileSys.Size > maxFileSystemSize {
			return status.Error(codes.PermissionDenied, "expansion capacity not allowed")
		}

		// Expand file system size
		_, err = filesystem.cs.api.UpdateFilesystem(filesystemID, fileSys)
		if err != nil {
			klog.Errorf("failed to update file system %v", err)
			return err
		}
	}

	// Expand Treeq size
	body := map[string]interface{}{"hard_capacity": capacity}
	_, err = filesystem.cs.api.UpdateTreeq(filesystemID, treeqID, body)
	if err != nil {
		klog.Errorf("failed to update treeq size %v", err)
		return
	}

	klog.V(2).Info("Treeq size updated successfully")
	return
}
