package common

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"github.com/infinidat/infinibox-csi-driver/log"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/go-logr/zerologr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var zlog = log.Get() // grab the logger for package use

// Global resource contains a sync.Mutex. Used to serialize iSCSI resource accesses.
var ExecCommand helper.Exec

type Commonservice struct {
	IboxAPI           iboxapi.Client
	API               api.Client
	storagePoolIDName map[int]string
	driverVersion     string
	AccessModesHelper helper.AccessModesHelper
	VolProto          *api.VolumeProtocolConfig
}

const (
	// for size conversion
	KIB int64 = 1024
	MIB int64 = KIB * 1024
	GIB int64 = MIB * 1024
	// gib100 int64 = gib * 100
	TIB int64 = GIB * 1024
	// tib100 int64 = tib * 100
	TOBEDELETED                = "host.k8s.to_be_deleted"
	HOST_ID_PUBLISH_CONTEXT    = "hostID"
	HOST_PORTS_PUBLISH_CONTEXT = "hostPorts"
	LUN_PUBLISH_CONTEXT        = "lun"
)

func BuildCommonService(config map[string]string, secretMap map[string]string, volumePrototype *api.VolumeProtocolConfig) (Commonservice, error) {
	commonService := Commonservice{}
	if config != nil {
		if len(secretMap) < 3 {
			zlog.Error().Msgf("Api client cannot be initialized without proper secrets")
			return commonService, errors.New("secrets are missing or not valid")
		}
		hostnameURL, err := url.Parse(secretMap[common.CredentialHostname])

		if err != nil {
			zlog.Error().Msgf("Error parsing IBox hostname: %s", err.Error())
			return commonService, errors.New("secret hostname is missing or not valid")
		}

		// check for scheme, add if missing.
		URLScheme := hostnameURL.Scheme

		var APIHost string
		if URLScheme == "" {
			zlog.Trace().Msgf("IBox Hostname is missing scheme, setting https as scheme")
			APIHost = "https://" + secretMap[common.CredentialHostname] + "/"
		} else {
			APIHost = hostnameURL.String()
		}

		// check for URI validity.
		hostnameURL, err = url.ParseRequestURI(APIHost)
		if err != nil {
			zlog.Error().Msgf("IBox hostname %s is invalid URI: %s", hostnameURL.String(), err.Error())
		} else {
			zlog.Trace().Msgf("IBox URL: %s", APIHost)
		}
		creds := iboxapi.Credentials{
			Username: secretMap[common.CredentialUsername],
			Password: secretMap[common.CredentialPassword],
			URL:      APIHost,
		}
		var iboxAPILog = zerologr.New(&zlog)

		iboxAPIClient := iboxapi.NewIboxClient(iboxAPILog, creds)
		commonService = Commonservice{
			API: &api.ClientService{
				SecretsMap: secretMap,
			},
			IboxAPI:  iboxAPIClient,
			VolProto: volumePrototype,
		}
		err = commonService.verifyAPIClient()
		if err != nil {
			zlog.Error().Msgf("API client not initialized, err: %v", err)
			return commonService, err
		}
		commonService.driverVersion = config["driverversion"]
		commonService.AccessModesHelper = helper.AccessMode{}
	}
	zlog.Trace().Msgf("buildCommonService commonservice configuration done. config %+v", config)
	return commonService, nil
}

func (cs *Commonservice) MapVolumeTohost(volumeID int, hostID int) (lunInfo *iboxapi.LunInfo, err error) {
	lunInfo, err = cs.IboxAPI.MapVolumeToHost(hostID, volumeID, -1)
	if err != nil {
		if strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
			lunInfo, err = cs.IboxAPI.GetLunByHostVolume(hostID, volumeID)
		}
		if err != nil {
			return lunInfo, err
		}
	}
	return lunInfo, nil
}

func (cs *Commonservice) UnmapVolumeFromHost(hostID, volumeID int) (err error) {
	_, err = cs.IboxAPI.UnMapVolumeFromHost(hostID, volumeID)
	if err != nil {
		// Ignore the following errors
		successMsg := fmt.Sprintf("Success: No need to unmap volume with ID %d from host with ID %d", volumeID, hostID)
		if strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			zlog.Debug().Msgf("%s, host not found", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			zlog.Debug().Msgf("%s, lun not found", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			zlog.Debug().Msgf("%s, volume not found", successMsg)
			return nil
		}
		return err
	}
	return nil
}

func (cs *Commonservice) AddPortForHost(hostID int, portType, portName string) error {
	_, err := cs.IboxAPI.AddHostPort(portType, portName, hostID)
	if err != nil && !strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
		zlog.Error().Msgf("failed to add host port with error %v", err)
		return err
	}
	return nil
}

/**
func (cs *Commonservice) AddChapSecurityForHost(hostID int, credentials map[string]string) error {
	_, err := cs.IboxApi.AddHostSecurity(credentials, hostID)
	if err != nil {
		zlog.Error().Msgf("failed to add authentication for host %d with error %v", hostID, err)
		return err
	}
	return nil
}
*/

func (cs *Commonservice) ValidateHost(hostName string) (*iboxapi.Host, error) {
	const functionName = "validateHost"
	zlog.Debug().Msgf("%s - Check if host available, create if not available", functionName)
	removeDomainName := os.Getenv(common.EnvVarRemoveDomainName)
	if removeDomainName != "" && removeDomainName == "true" {
		shortName := strings.Split(hostName, ".")
		zlog.Debug().Msgf("%s - REMOVE_DOMAIN_NAME set to true, %s resulting in %s", functionName, hostName, shortName[0])
		hostName = shortName[0]
	}
	host, err := cs.IboxAPI.GetHostByName(hostName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s - Creating host with name: %s", functionName, hostName)
			host, err = cs.IboxAPI.CreateHost(hostName)
			if err != nil {
				e := fmt.Errorf("%s - error failed to create host %s with error %s", functionName, hostName, err)
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}

			metadata := map[string]interface{}{
				common.CSICreatedHost: true,
			}
			_, err = cs.IboxAPI.PutMetadata(host.ID, metadata)
			if err != nil {
				e := fmt.Errorf("%s - error creating host metadata : %s id %d error : %v", functionName, hostName, host.ID, err)
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
		} else {
			e := fmt.Errorf("validateHost - GetHostByName - hostname %s error %s", hostName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return host, nil
}

func (cs *Commonservice) GetCSIResponse(vol *iboxapi.Volume, req *csi.CreateVolumeRequest) *csi.Volume {
	zlog.Debug().Msgf("getCSIResponse called with volume %+v", vol)
	storagePoolName := vol.PoolName
	if storagePoolName == "" {
		storagePoolName = cs.getStoragePoolNameFromID(vol.PoolID)
	}
	// Make the additional volume volumeAttributes
	volumeAttributes := map[string]string{
		"ID":              strconv.Itoa(vol.ID),
		"Name":            vol.Name,
		"StoragePoolID":   strconv.Itoa(vol.PoolID),
		"StoragePoolName": storagePoolName,
		"CreationTime":    time.Unix(vol.CreatedAt, 0).String(),
		"targetWWNs":      req.GetParameters()["targetWWNs"],
	}
	volume := &csi.Volume{
		VolumeId:      strconv.Itoa(vol.ID),
		CapacityBytes: vol.Size,
		VolumeContext: volumeAttributes,
		ContentSource: req.GetVolumeContentSource(),
	}
	return volume
}

func (cs *Commonservice) GetNetworkSpaceIP(networkSpace string) (string, error) {
	const functionName = "getNetworkSpaceIP"
	existingNetworkSpace, err := cs.IboxAPI.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return "", err
	}
	if len(existingNetworkSpace.Portals) == 0 {
		return "", fmt.Errorf("%s - error IP address not found", functionName)
	}

	index := GetRandomIndex(len(existingNetworkSpace.Portals))
	return existingNetworkSpace.Portals[index].IPAddress, nil
}

func GetRandomIndex(maxIndex int) int {
	var minIndex int
	index := rand.Intn(maxIndex-minIndex) + minIndex
	return index
}

func (cs *Commonservice) GetCreatedBy() string {
	var createdBy string
	createdBy = "CSI/" + cs.driverVersion
	k8version := getClusterVersion()
	if k8version != "" {
		createdBy = "CSI/" + k8version + "/" + cs.driverVersion
	}
	return createdBy
}

func getClusterVersion() string {
	cl, err := clientgo.BuildClient()
	if err != nil {
		return ""
	}
	version, _ := cl.GetClusterVerion()
	return version
}

func (cs *Commonservice) PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		zlog.Debug().Msgf("path exists: %s", path)
		return true, nil
	} else if os.IsNotExist(err) {
		zlog.Debug().Msgf("path does not exist: %s", path)
		return false, nil
	} else if cs.IsCorruptedMnt(err) {
		zlog.Debug().Msgf("path is corrupted: %s", path)
		return true, err
	}
	zlog.Debug().Msgf("unable to validate path: %s", path)
	return false, err
}

func (cs *Commonservice) IsCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pathError := err.(type) {
	case *os.PathError:
		underlyingError = pathError.Err
	case *os.LinkError:
		underlyingError = pathError.Err
	case *os.SyscallError:
		underlyingError = pathError.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO
}

func CopyRequestParameters(parameters, out map[string]string) {
	for key, val := range parameters {
		if val != "" {
			out[key] = val
		}
	}
}

func DetermineSSDValue(ssdStorageClassParameter string, poolName string, client iboxapi.Client) (ssdValue bool, err error) {
	var valueProvidedInStorageClass bool
	if ssdStorageClassParameter != "" {
		valueProvidedInStorageClass = true
	}

	if valueProvidedInStorageClass {
		ssdValue, err = strconv.ParseBool(ssdStorageClassParameter)
		if err != nil {
			return ssdValue, err
		}
		zlog.Debug().Msgf("setting ssd value %t from storage class parameter", ssdValue)
		return ssdValue, nil
	}

	// get the ssd value from the pool
	pool, err := client.GetPoolByName(poolName)
	if err != nil {
		zlog.Error().Msgf("determineSSDValue error %s", err.Error())
		return ssdValue, err
	}
	zlog.Debug().Msgf("setting ssd value %t from pool", pool.SsdEnabled)
	return pool.SsdEnabled, nil
}

func (sh StorageService) ValidateIPAddress(ip string, port int) (err error) {
	start := time.Now()

	ipAndPort := ip + ":" + strconv.Itoa(port)
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.Dial("tcp", ipAndPort)
	if conn != nil {
		if err := conn.Close(); err != nil {
			zlog.Error().Msgf("error in Close() %s", err.Error())
		}
	}
	elapsed := time.Since(start)

	if err != nil {
		zlog.Error().Msgf("error dialing IP address %s - %s time: %s", ipAndPort, err.Error(), elapsed)
		return err
	}
	zlog.Debug().Msgf("IP address %s is reachable, time: %s", ipAndPort, elapsed)
	return nil
}

func PortalMounter(portal string) string {
	if !strings.Contains(portal, ":") {
		portal += ":3260"
	}
	return portal
}

// Used for debugging. Log a path, found by debugWalkDir, to log.
func DebugLogPath(path string, _ os.FileInfo, err error) error {
	if err != nil {
		zlog.Err(err)
		return err
	}
	zlog.Trace().Msgf("found path %s", path)
	return nil
}

// Used for debugging. For given walk_path, log all files found within.
func DebugWalkDir(walkPath string) (err error) {
	zlog.Trace().Msgf("walkPath %s", walkPath)
	err = filepath.Walk(walkPath, DebugLogPath)
	if err != nil {
		zlog.Err(err)
		return err
	}
	return nil
}

func GetHostInfo(secrets map[string]string, client iboxapi.Client) (iboxInfo string) {
	sys, _ := client.GetSystem()
	var serialNumber int
	if sys != nil {
		serialNumber = sys.SerialNumber
	}
	return fmt.Sprintf(" - ibox %s (%d)", secrets[common.CredentialHostname], serialNumber)
}

func ValidatePublishContext(publishContext map[string]string) (hostID int, ports string, err error) {
	hostIDString := publishContext[HOST_ID_PUBLISH_CONTEXT]
	hostID, err = strconv.Atoi(hostIDString)
	if err != nil {
		err := fmt.Errorf("hostID string '%s' is not valid host ID: %v", hostIDString, err)
		zlog.Err(err)
		return 0, "", status.Error(codes.Internal, err.Error())
	}

	if hostID < 1 {
		e := fmt.Errorf("hostID %d is not valid host ID", hostID)
		return 0, "", status.Error(codes.Internal, e.Error())
	}

	ports = publishContext[HOST_PORTS_PUBLISH_CONTEXT]

	return hostID, ports, nil
}

func HostCleanup(iboxClient iboxapi.Client, hostID int, hostName string) error {
	meta, err := iboxClient.GetMetadata(hostID)
	if err != nil {
		e := fmt.Errorf("hostCleanup: failed to get metadata for host ID %d. Error: %v", hostID, err)
		zlog.Err(e)
		return status.Error(codes.Internal, e.Error())
	}
	var createdByCSI bool
	for i := range meta {
		if meta[i].Key == common.CSICreatedHost {
			createdByCSI = true
		}
	}

	if createdByCSI {
		response, err := iboxClient.DeleteHost(hostID)
		if err != nil {
			re, ok := err.(*iboxapi.APIError)
			if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
				zlog.Debug().Msgf("hostCleanup: will not delete, host not found %d %+v", hostID, response)
			} else {
				zlog.Error().Msgf("hostCleanup: failed to delete host with error %v", err)
				return status.Error(codes.Internal, err.Error())
			}
		}
		zlog.Debug().Msgf("hostCleanup: deleted host on ibox because it was created by CSI host %d %s", hostID, hostName)
	} else {
		zlog.Debug().Msgf("hostCleanup: not deleting host because it was not created by CSI host %d", hostID)
	}
	return nil
}

func DetermineHostName(nodeID string) (hostName string, err error) {
	if nodeID == "" {
		return "", status.Error(codes.InvalidArgument, "node ID empty")
	}
	nodeNameIP := strings.Split(nodeID, "$$")
	if len(nodeNameIP) != 2 {
		return "", status.Error(codes.NotFound, fmt.Sprintf("node ID: %s not found", nodeID))
	}
	hostName = nodeNameIP[0]

	removeDomainName := os.Getenv(common.EnvVarRemoveDomainName)
	if removeDomainName == "true" {
		shortName := strings.Split(hostName, ".")
		if len(shortName) > 0 {
			zlog.Debug().Msgf("REMOVE_DOMAIN_NAME set to true, %s resulting in %s", hostName, shortName[0])
			hostName = shortName[0]
		}
	}
	return hostName, nil
}

func ValidateVolumeID(volumeIDString string) (volprotoconf api.VolumeProtocolConfig, err error) {
	zlog.Debug().Msgf("ValidateVolumeID volumeIDString %s", volumeIDString)

	if volumeIDString == "" {
		return volprotoconf, errors.New("volume Id string is empty")
	}
	volproto := strings.Split(volumeIDString, "$$")
	if len(volproto) != 2 {
		return volprotoconf, errors.New("volume Id and other details not found")
	}

	if volproto[0] == "" {
		return volprotoconf, errors.New("volume Id in volproto is empty")
	}

	if volproto[1] == "" {
		return volprotoconf, errors.New("volume storagetype in volproto is empty")
	}
	volprotoconf.StorageType = volproto[1]

	// treeq is a special formatting case
	if volprotoconf.StorageType == common.ProtocolTreeq {
		// example: volproto[0] == 2942184#20000
		tmp := strings.Split(volproto[0], "#")
		if len(tmp) != 2 {
			return volprotoconf, fmt.Errorf("treeq volume not correctly formatted %s", volproto[0])
		}
		volprotoconf.VolumeID, err = strconv.Atoi(tmp[0])
		if err != nil {
			return volprotoconf, err
		}
		zlog.Debug().Msgf("ValidateVolumeID treeq VolumeID %d treeqID %s", volprotoconf.VolumeID, tmp[1])

		volprotoconf.TreeqID, err = strconv.Atoi(tmp[1])
		if err != nil {
			return volprotoconf, fmt.Errorf("volume treeq id parse error %s on %s", err.Error(), tmp[1])
		}
		return volprotoconf, nil
	}

	// for any other protocol than treeq
	volprotoconf.VolumeID, err = strconv.Atoi(volproto[0])
	if err != nil {
		e := fmt.Errorf("failed to validate volume id %s, err: %v", volproto[0], err)
		zlog.Err(e)
		return volprotoconf, errors.New("volume id in volproto is not an integer")
	}

	return volprotoconf, nil
}

func StringToLines(s string) (lines []string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	err = scanner.Err()
	return
}

func (cs *Commonservice) getStoragePoolNameFromID(poolID int) string {
	const functionName = "getStoragePoolNameFromID"
	zlog.Debug().Msgf("%s called with storagepoolid %d", functionName, poolID)
	storagePoolName := cs.storagePoolIDName[poolID]
	if storagePoolName == "" {
		pool, err := cs.IboxAPI.GetPoolByID(poolID)
		if err == nil {
			storagePoolName = pool.Name
			cs.storagePoolIDName[poolID] = pool.Name
		} else {
			zlog.Error().Msgf("%s - Could not find StoragePool: %d", functionName, poolID)
		}
	}
	return storagePoolName
}
func (cs *Commonservice) verifyAPIClient() error {
	zlog.Trace().Msgf("verifying api client")
	c, err := cs.API.NewClient()
	if err != nil {
		zlog.Error().Msgf("api client is not working.")
		return errors.New("failed to create rest client")
	}
	cs.API = c
	zlog.Trace().Msgf("api client is verified.")
	return nil
}
