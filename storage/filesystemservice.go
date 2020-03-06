package storage

import (
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"path"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//treeq constants
const (
	PROVISIONTYPE          = "provision_type"
	MAXTREEQSPERFILESYSTEM = "max_treeqs_per_filesystem"
	MAXFILESYSTEMS         = "max_filesystems"
	MAXFILESYSTEMSIZE      = "max_filesystem_size"
	UNIXPERMISSION         = "nfs_unix_permissions"
	FSPREFIX               = "fs_prefix"

	//Treeq count
	TREEQCOUNT = "host.k8s.treeqs"
)

// service type
const (
	NFSTREEQ = "nfs_treeq"
	NFS      = "nfs"
)

//Operation declare for treeq count operation
type ACTION int

const (
	//Increment operation
	IncrementTreeqCount ACTION = 1 + iota
	//decrement operation
	DecrementTreeqCount
	NONE
)

//FilesystemService file system services
type FilesystemService struct {
	uniqueID  int64
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

//FileSystemInterface interface
type FileSystemInterface interface {
	validateTreeqParameters(config map[string]string) (bool, map[string]string)
	CreateTreeqVolume(config map[string]string, capacity int64, pvName string) (map[string]string, error)
	DeleteTreeqVolume(filesystemID, treeqID int64) error
	UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxSize string) error
}

func (filesystem *FilesystemService) getExpectedFileSystemID() (filesys *api.FileSystem, err error) {

	maxFileSystemSize, err := filesystem.maxFileSize()
	if err != nil {
		log.Error(err)
		return
	}
	if filesystem.capacity > maxFileSystemSize {
		log.Errorf("Can't allowed to create treeq of size %d", filesystem.capacity)
		log.Errorf("Max allowed filesytem size %d", maxFileSystemSize)
		err = errors.New("Request treeq size is greater than allowed max_filesystem_size")
		return
	}

	poolID, err := filesystem.cs.api.GetStoragePoolIDByName(filesystem.configmap["pool_name"])
	if err != nil {
		log.Errorf("fail to get poolID from poolName %s", filesystem.configmap["pool_name"])
		return
	}
	filesystem.poolID = poolID
	page := 1
	for {
		fsMetaData, poolErr := filesystem.cs.api.GetFileSystemsByPoolID(poolID, page)
		if poolErr != nil {
			log.Errorf("fail to get filesystems from poolID %d and page no %d error %v", poolID, page, err)
			err = errors.New("fail to get filesystems from poolName " + filesystem.configmap["pool_name"])
			return
		}
		if fsMetaData != nil && len(fsMetaData.FileSystemArry) == 0 {
			log.Debugf("NO filesystem found.filesystem array is empty")
			return
		}
		for _, fs := range fsMetaData.FileSystemArry {
			if fs.Size+filesystem.capacity < maxFileSystemSize {
				treeqCnt, treeqCnterr := filesystem.cs.api.GetFilesytemTreeqCount(fs.ID)
				if treeqCnterr != nil {
					log.Errorf("fail to get treeq count of filesystemID %d error %v", fs.ID, err)
					err = errors.New("fail to get treeq count of filesystemID " + strconv.FormatInt(fs.ID, 10))
					return
				}
				if treeqCnt < filesystem.getAllowedCount(MAXTREEQSPERFILESYSTEM) {
					filesystem.treeqCnt = treeqCnt
					log.Debugf("filesystem found to create treeQ,filesystemID %d", fs.ID)
					exportErr := filesystem.getExportPath(fs.ID) //fetch export path and set to filesystem exportPath
					if exportErr != nil {
						err = exportErr
					}
					filesys = &fs
					return
				}
			}
		} //inner for loop closed
		if fsMetaData.Filemetadata.PagesTotal == fsMetaData.Filemetadata.Page {
			break
		}
		page++ //check the file system on next page
	} //outer for loop closed
	log.Debugf("NO filesystem found to create treeQ")
	return
}

func (filesystem *FilesystemService) setParameter(config map[string]string, capacity int64, pvName string) {
	filesystem.pVName = pvName
	filesystem.configmap = config
	filesystem.capacity = capacity
	filesystem.exportpath = "/" + filesystem.pVName
}

//CreateTreeqVolume create volumne method
func (filesystem *FilesystemService) CreateTreeqVolume(config map[string]string, capacity int64, pvName string) (treeqVolume map[string]string, err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while creating treeq method " + fmt.Sprint(res))
		}
	}()
	treeqVolume = make(map[string]string)
	treeqVolume["storage_protocol"] = config["storage_protocol"]
	treeqVolume["nfs_mount_options"] = config["nfs_mount_options"]
	filesystem.setParameter(config, capacity, pvName)

	ipAddress, err := filesystem.cs.getNetworkSpaceIP(strings.Trim(config["network_space"], " "))
	if err != nil {
		log.Errorf("fail to get networkspace ipaddress %v", err)
		return
	}
	filesystem.ipAddress = ipAddress
	log.Debugf("getNetworkSpaceIP ipAddress %s", ipAddress)

	filesys, err := filesystem.getExpectedFileSystemID()
	if err != nil {
		return
	}
	var filesystemID int64
	if filesys == nil { // if pool is empty or no file system found to createTreeq
		err = filesystem.createFileSystem()
		if err != nil {
			log.Errorf("fail to create fileSystem %v", err)
			return
		}
		err = filesystem.createExportPathAndAddMetadata()
		if err != nil {
			log.Errorf("fail to create export and metadata %v", err)
			return
		}
		filesystemID = filesystem.fileSystemID
	} else {
		filesystemID = filesys.ID
	}
	//create treeq
	treeqResponse, createTreeqerr := filesystem.cs.api.CreateTreeq(filesystemID, filesystem.getTreeParameters())
	if createTreeqerr != nil {
		log.Errorf("fail to create treeq  %s error %v", filesystem.pVName, err)
		if filesys == nil { //if the file system created at the time of creating first treeq ,then delete the complete filesystem with export and metata
			deleteFilesystemErr := filesystem.cs.api.DeleteFileSystemComplete(filesystemID)
			if deleteFilesystemErr != nil {
				log.Errorf("fail to delete filesystem ,filesystemID = %d", filesystemID)
			}
		}
		err = errors.New("fail to Create Treeq")
		return
	}
	treeqVolume["ID"] = strconv.FormatInt(filesystemID, 10)
	treeqVolume["TREEQID"] = strconv.FormatInt(treeqResponse.ID, 10)
	treeqVolume["ipAddress"] = filesystem.ipAddress
	treeqVolume["volumePath"] = path.Join(filesystem.exportpath, treeqResponse.Path)

	//if AttachMetadataToObject - fail to add metadata then delete the created treeq
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while update metadata" + fmt.Sprint(res))
		}
		if err != nil && filesystem.fileSystemID != 0 {
			log.Infof("Seemes to be some problem reverting treeq: %s", filesystem.pVName)
			filesystem.cs.api.DeleteTreeq(filesystem.fileSystemID, treeqResponse.ID)
		}
	}()

	treeqCount := filesystem.treeqCnt + 1
	_, updateTreeqErr := filesystem.UpdateTreeqCnt(filesystemID, NONE, treeqCount)
	if updateTreeqErr != nil {
		err = errors.New("fail to increment treeq count as metadata")
		return
	}

	//if UpdateFilesystem - is fail then descrement the tree count from metadata
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while update file size" + fmt.Sprint(res))
		}
		if err != nil && filesystemID != 0 {
			log.Infof("Seemes to be some problem reverting treeqcount")
			filesystem.UpdateTreeqCnt(filesystemID, DecrementTreeqCount, 0)
		}
	}()

	// if new file system is created ,while creating the treeq, then not need to update size
	if filesys != nil {
		var updateFileSys api.FileSystem
		updateFileSys.Size = filesys.Size + filesystem.capacity
		_, updateFileSizeErr := filesystem.cs.api.UpdateFilesystem(filesystemID, updateFileSys)
		if updateFileSizeErr != nil {
			log.Errorf("fail to update File Size %v", err)
			err = errors.New("fail to update files size")
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
			log.Infof("Seemes to be some problem reverting filesystem: %s", filesystem.pVName)
			filesystem.cs.api.DeleteFileSystem(filesystem.fileSystemID)
		}
	}()

	err = filesystem.createExportPath()
	if err != nil {
		log.Errorf("fail to export path %v", err)
		return
	}
	log.Debugf("export path created for filesystem: %s", filesystem.pVName)

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while AttachMetadata directory" + fmt.Sprint(res))
		}
		if err != nil && filesystem.exportID != 0 {
			log.Infoln("Seemes to be some problem reverting created export id:", filesystem.exportID)
			filesystem.cs.api.DeleteExportPath(filesystem.exportID)
		}
	}()
	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = filesystem.pVName
	metadata["host.created_by"] = filesystem.cs.GetCreatedBy()

	_, err = filesystem.cs.api.AttachMetadataToObject(filesystem.fileSystemID, metadata)
	if err != nil {
		log.Errorf("fail to attach metadata for fileSystem : %s", filesystem.pVName)
		log.Errorf("error to attach metadata %v", err)
		return
	}
	log.Debugf("metadata attached successfully for filesystem %s", filesystem.pVName)
	return
}

func (filesystem *FilesystemService) createFileSystem() (err error) {
	fileSystemCnt, err := filesystem.cs.api.GetFileSystemCount()
	if err != nil {
		log.Errorf("fail to get the filesystem count from Ibox %v", err)
		return
	}
	if fileSystemCnt >= filesystem.getAllowedCount(MAXFILESYSTEMS) {
		log.Debugf("Max filesystem allowed on Ibox %v", filesystem.getAllowedCount(MAXFILESYSTEMS))
		log.Debugf("Current filesystem count on Ibox %v", fileSystemCnt)
		log.Errorf("Ibox not allowed to create new file system")
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
	if prefix, ok := filesystem.configmap[FSPREFIX]; ok {
		pvSplit := strings.Split(filesystem.pVName, "-")
		if len(pvSplit) == 2 {
			treeqFileSystemName = prefix + pvSplit[1]
			filesystem.exportpath = "/" + treeqFileSystemName
		}
	}

	mapRequest["name"] = treeqFileSystemName
	mapRequest["ssd_enabled"] = ssd
	mapRequest["provtype"] = strings.ToUpper(filesystem.configmap["provision_type"])
	mapRequest["size"] = filesystem.capacity
	fileSystem, err := filesystem.cs.api.CreateFilesystem(mapRequest)
	if err != nil {
		log.Errorf("fail to create filesystem %s", filesystem.pVName)
		return
	}
	filesystem.fileSystemID = fileSystem.ID
	log.Debugf("filesystem Created %s", filesystem.pVName)
	return
}

func (filesystem *FilesystemService) createExportPath() (err error) {
	permissionsMapArray, err := getPermission(filesystem.configmap["nfs_export_permissions"])
	if err != nil {
		return
	}
	var permissionsput []map[string]interface{}
	for _, pass := range permissionsMapArray {
		access := pass["access"].(string)
		var rootsq bool
		_, ok := pass["no_root_squash"].(string)
		if ok {
			rootsq, err = strconv.ParseBool(pass["no_root_squash"].(string))
			if err != nil {
				log.Debug("fail to cast no_root_squash value in export permission . setting default value 'true' ")
				rootsq = true
			}
		} else {
			rootsq = pass["no_root_squash"].(bool)
		}
		client := pass["client"].(string)
		permissionsput = append(permissionsput, map[string]interface{}{"access": access, "no_root_squash": rootsq, "client": client})
	}
	var exportFileSystem api.ExportFileSys
	exportFileSystem.FilesystemID = filesystem.fileSystemID
	exportFileSystem.Transport_protocols = "TCP"
	exportFileSystem.Privileged_port = true
	exportFileSystem.Export_path = filesystem.exportpath
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsput...)
	exportResp, err := filesystem.cs.api.ExportFileSystem(exportFileSystem)
	if err != nil {
		log.Errorf("fail to create export path of filesystem %s", filesystem.pVName)
		return
	}
	filesystem.exportID = exportResp.ID
	filesystem.exportBlock = exportResp.ExportPath
	return
}

func (filesystem *FilesystemService) validateTreeqParameters(config map[string]string) (bool, map[string]string) {
	compulsaryFields := []string{"pool_name", "network_space", "nfs_export_permissions"}
	validationStatus := true
	validationStatusMap := make(map[string]string)
	for _, param := range compulsaryFields {
		if config[param] == "" {
			validationStatusMap[param] = param + " value missing"
			validationStatus = false
		}
	}
	log.Debug("parameter Validation completed")
	return validationStatus, validationStatusMap
}

func getDefaultValues() map[string]string {
	defaultConfigMap := make(map[string]string)
	defaultConfigMap[PROVISIONTYPE] = "thin"
	defaultConfigMap[MAXTREEQSPERFILESYSTEM] = "1000"
	defaultConfigMap[MAXFILESYSTEMS] = "1000"
	defaultConfigMap[MAXFILESYSTEMSIZE] = "100tib"
	defaultConfigMap[UNIXPERMISSION] = "750"
	return defaultConfigMap
}

func (filesystem *FilesystemService) getAllowedCount(key string) int {
	var allowedCnt int = 0
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
			log.Errorf("fail to convert MAXFILESYSTEMSIZE %s to byte", maxfilesize)
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
				log.Errorf("fail to convert the %s to bytes", size)
				return
			}
			bytes = sizeUnit * unit
			return
		}
	}
	err = errors.New("Unexpected maxfilesystemsize .Expected size formate shoude be in formate of gib,tib")
	return
}

func (filesystem *FilesystemService) getTreeParameters() map[string]interface{} {
	treeqParameter := make(map[string]interface{})
	treeqParameter["path"] = path.Join("/", filesystem.pVName)
	treeqParameter["name"] = filesystem.pVName
	treeqParameter["hard_capacity"] = filesystem.capacity
	treeqParameter["mode"] = filesystem.getTreeModePermission()
	return treeqParameter
}

func (filesystem *FilesystemService) getTreeModePermission() string {
	if unixPermission, ok := filesystem.configmap[UNIXPERMISSION]; ok {
		return unixPermission
	}
	values := getDefaultValues()
	return values[UNIXPERMISSION]
}

func (filesystem *FilesystemService) getExportPath(filesystemID int64) error {
	exportResponse, exportErr := filesystem.cs.api.GetExportByFileSystem(filesystemID)
	if exportErr != nil {
		log.Errorf("fail to create export path of filesystem %d", filesystemID)
		return exportErr
	}
	for _, export := range *exportResponse {
		filesystem.exportpath = export.ExportPath
		break
	}
	return nil
}

//treeq delete

func isTreeQEmpty(treeq api.Treeq) bool {
	if treeq.UsedCapacity > 0 {
		return false
	}
	return true
}

//DeleteNFSVolume delete volume method
func (filesystem *FilesystemService) DeleteTreeqVolume(filesystemID, treeqID int64) (err error) {

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while deleting treeq " + fmt.Sprint(res))
			return
		}
	}()
	//1.treeq exist or not checked
	var treeq *api.Treeq
	treeq, err = filesystem.cs.api.GetTreeq(filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			err = errors.New("Treeq does not exist on infinibox")
			return nil
		}
		log.Errorf("Error occured while getting treeq: %s", err)
		return
	}

	//2.if treeq has usedcapacity >0 then..
	if !isTreeQEmpty(*treeq) {
		log.Error("Can't delete NFS-treeq PV with data")
		err = errors.New("Can't delete NFS-treeq PV with data")
		return
	}
	//3 first decremnt the treeq count to recover
	// In case of 1 - we are deleting the file system,
	treeqCnt, err := filesystem.UpdateTreeqCnt(filesystemID, DecrementTreeqCount, 0)
	if err != nil {
		log.Error("fail to update treeq count")
		return
	}

	//4.delete the treeq
	_, err = filesystem.cs.api.DeleteTreeq(filesystemID, treeqID)
	if err != nil {
		log.Error("fail to delete treeq")
		filesystem.UpdateTreeqCnt(filesystemID, IncrementTreeqCount, 0)
		return
	}

	//5.Delete file system if all treeq are delete
	if treeqCnt == 0 { // measn all tree are delete. then delete the complete filesystem with exportPath ,metadata..etc
		err = filesystem.cs.api.DeleteFileSystemComplete(filesystemID)
		if err != nil {
			log.Errorf("fail to delete filesystem filesystemID %d error %v", filesystemID, err)
			return
		}
	}
	log.Debug("Treeq deleted successfully")
	return
}

//UpdateTreeqCnt method
func (filesystem *FilesystemService) UpdateTreeqCnt(fileSystemID int64, action ACTION, treeqCnt int) (treeqCount int, err error) {
	if treeqCnt == 0 {
		treeqCnt, err = filesystem.cs.api.GetFilesytemTreeqCount(fileSystemID)
		if err != nil {
			return
		}
		log.Debugf("treeq count of fileSystemID: %d", fileSystemID)
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
		log.Errorf("Error occured updating treeq count to filesystemID : %d error %v", fileSystemID, err)
		return
	}
	treeqCount = treeqCnt
	log.Debugf("treeq count updated successfully of fileSystemID: %d", fileSystemID)
	return
}

//UpdateTreeqVolume Upadate volume size method
func (filesystem *FilesystemService) UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxSize string) (err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("Error while updating treeq " + fmt.Sprint(res))
			return
		}
	}()

	//Get Filesystem
	fileSystemResponse, err := filesystem.cs.api.GetFileSystemByID(filesystemID)
	if err != nil {
		log.Errorf("Failed to get file system %v", err)
		return
	}

	//Get a treeq
	treeq, err := filesystem.cs.api.GetTreeq(filesystemID, treeqID)
	if err != nil {
		if strings.Contains(err.Error(), "TREEQ_ID_DOES_NOT_EXIST") {
			err = errors.New("Treeq does not exist on infinibox")
			return nil
		}
		log.Errorf("Error occured while getting treeq: %s", err)
		return
	}

	// Get sum of all the treeq size of filesystem
	totalTreeqSize, err := filesystem.cs.api.GetTreeqSizeByFileSystemID(filesystemID)
	if err != nil {
		log.Error("Failed to get sum of all the treeq size of a filesystem")
		return
	}

	needToIncreaseSize := capacity - treeq.HardCapacity
	if totalTreeqSize+needToIncreaseSize > fileSystemResponse.Size {
		configMap := make(map[string]string)
		configMap[MAXFILESYSTEMSIZE] = maxSize
		filesystem.configmap = configMap
		//Get Maximum filesystem size
		maxFileSystemSize, err := filesystem.maxFileSize()
		if err != nil {
			log.Error(err)
			return err
		}

		var fileSys api.FileSystem
		freeSpace := fileSystemResponse.Size - totalTreeqSize
		increaseFileSizeBy := needToIncreaseSize - freeSpace
		fileSys.Size = fileSystemResponse.Size + increaseFileSizeBy
		if fileSys.Size > maxFileSystemSize {
			return status.Error(codes.PermissionDenied, "Given capacity for expansion is not allowed")
		}

		// Expand file system size
		_, err = filesystem.cs.api.UpdateFilesystem(filesystemID, fileSys)
		if err != nil {
			log.Errorf("Failed to update file system %v", err)
			return err
		}
	}

	// Expand Treeq size
	body := map[string]interface{}{"hard_capacity": capacity}
	_, err = filesystem.cs.api.UpdateTreeq(filesystemID, treeqID, body)
	if err != nil {
		log.Errorf("Failed to update treeq size %v", err)
		return
	}

	log.Infoln("Treeq size updated successfully")
	return
}
