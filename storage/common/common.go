package common

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
	ToBeDeleted             = "host.k8s.to_be_deleted"
	HostIDPublishContext    = "hostID"
	HostPortsPublishContext = "hostPorts"
	LunPublishContext       = "lun"
)

func BuildCommonService(config map[string]string, secrets map[string]string, volumePrototype *api.VolumeProtocolConfig) (Commonservice, error) {
	commonService := Commonservice{}
	if config != nil {
		if len(secrets) < 3 {
			slog.Error("Api client cannot be initialized without proper secrets")
			return commonService, errors.New("secrets are missing or not valid")
		}
		hostnameURL, err := url.Parse(secrets[common.CredentialHostname])

		if err != nil {
			slog.Error("Error parsing IBox hostname", "error", err.Error())
			return commonService, errors.New("secret hostname is missing or not valid")
		}

		// check for scheme, add if missing.
		URLScheme := hostnameURL.Scheme
		ctx := context.Background()

		var APIHost string
		if URLScheme == "" {
			slog.Log(ctx, common.LevelTrace, "IBox Hostname is missing scheme, setting https as scheme")
			APIHost = "https://" + secrets[common.CredentialHostname] + "/"
		} else {
			APIHost = hostnameURL.String()
		}

		// check for URI validity.
		hostnameURL, err = url.ParseRequestURI(APIHost)
		if err != nil {
			slog.Error("IBox hostname is invalid URI", "url", hostnameURL.String(), "error", err.Error())
		} else {
			slog.Log(ctx, common.LevelTrace, "IBox URL", "url", APIHost)
		}
		creds := iboxapi.Credentials{
			Username: secrets[common.CredentialUsername],
			Password: secrets[common.CredentialPassword],
			URL:      APIHost,
		}

		iboxAPIClient := iboxapi.NewIboxClient(creds)
		commonService = Commonservice{
			API: &api.ClientService{
				SecretsMap: secrets,
			},
			IboxAPI:  iboxAPIClient,
			VolProto: volumePrototype,
		}
		err = commonService.verifyAPIClient()
		if err != nil {
			slog.Error("API client not initialized", "error", err)
			return commonService, err
		}
		commonService.driverVersion = config["driverversion"]
		commonService.AccessModesHelper = helper.AccessMode{}
	}
	slog.Log(context.Background(), common.LevelTrace, "buildCommonService commonservice configuration done.", "config", config)
	return commonService, nil
}

func (cs *Commonservice) MapVolumeTohost(ctx context.Context, volumeID int, hostID int) (lunInfo *iboxapi.LunInfo, err error) {
	lunInfo, err = cs.IboxAPI.MapVolumeToHost(ctx, hostID, volumeID, -1)
	if err != nil {
		if strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
			lunInfo, err = cs.IboxAPI.GetLunByHostVolume(ctx, hostID, volumeID)
		}
		if err != nil {
			return lunInfo, err
		}
	}
	return lunInfo, nil
}

func (cs *Commonservice) UnmapVolumeFromHost(ctx context.Context, hostID, volumeID int) (err error) {
	_, err = cs.IboxAPI.UnMapVolumeFromHost(ctx, hostID, volumeID)
	if err != nil {
		// Ignore the following errors
		successMsg := fmt.Sprintf("Success: No need to unmap volume with ID %d from host with ID %d", volumeID, hostID)
		if strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			slog.Debug("host not found", "successMsg", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			slog.Debug("lun not found", "successMsg", successMsg)
			return nil
		} else if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			slog.Debug("volume not found", "successMsg", successMsg)
			return nil
		}
		return err
	}
	return nil
}

func (cs *Commonservice) AddPortForHost(ctx context.Context, hostID int, portType, portName string) error {
	_, err := cs.IboxAPI.AddHostPort(ctx, portType, portName, hostID)
	if err != nil && !strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
		slog.Error("failed to add host port with error", "error", err)
		return err
	}
	return nil
}

/**
func (cs *Commonservice) AddChapSecurityForHost(hostID int, credentials map[string]string) error {
	_, err := cs.IboxApi.AddHostSecurity(credentials, hostID)
	if err != nil {
		slog.Error()("failed to add authentication for host %d with error %v", hostID, err)
		return err
	}
	return nil
}
*/

func (cs *Commonservice) ValidateHost(ctx context.Context, hostName string) (*iboxapi.Host, error) {
	slog.Debug("Check if host available, create if not available")
	removeDomainName := os.Getenv(common.EnvVarRemoveDomainName)
	if removeDomainName != "" && removeDomainName == "true" {
		shortName := strings.Split(hostName, ".")
		slog.Debug("REMOVE_DOMAIN_NAME set to true, resulting in", "host", hostName, "short", shortName[0])
		hostName = shortName[0]
	}
	host, err := cs.IboxAPI.GetHostByName(ctx, hostName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("Creating host", "name", hostName)
			host, err = cs.IboxAPI.CreateHost(ctx, hostName)
			if err != nil {
				e := fmt.Errorf("error failed to create host %s with error %s", hostName, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}

			metadata := map[string]interface{}{
				common.CSICreatedHost: true,
			}
			_, err = cs.IboxAPI.PutMetadata(ctx, host.ID, metadata)
			if err != nil {
				e := fmt.Errorf("error creating host metadata : %s id %d error : %v", hostName, host.ID, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
		} else {
			e := fmt.Errorf("validateHost - GetHostByName - hostname %s error %s", hostName, err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return host, nil
}

func (cs *Commonservice) GetCSIResponse(ctx context.Context, vol *iboxapi.Volume, req *csi.CreateVolumeRequest) *csi.Volume {
	slog.Log(ctx, common.LevelTrace, "getCSIResponse called", "volume", vol)
	storagePoolName := vol.PoolName
	if storagePoolName == "" {
		storagePoolName = cs.getStoragePoolNameFromID(ctx, vol.PoolID)
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

func (cs *Commonservice) GetNetworkSpaceIP(ctx context.Context, networkSpace string) (string, error) {
	existingNetworkSpace, err := cs.IboxAPI.GetNetworkSpaceByName(ctx, networkSpace)
	if err != nil {
		return "", err
	}
	if len(existingNetworkSpace.Portals) == 0 {
		return "", fmt.Errorf("error IP address not found")
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
		slog.Debug("path exists", "path", path)
		return true, nil
	} else if os.IsNotExist(err) {
		slog.Debug("path does not exist", "path", path)
		return false, nil
	} else if cs.IsCorruptedMnt(err) {
		slog.Debug("path is corrupted", "path", path)
		return true, err
	}
	slog.Debug("unable to validate path", "path", path)
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

func DetermineSSDValue(ctx context.Context, ssdStorageClassParameter string, poolName string, client iboxapi.Client) (ssdValue bool, err error) {
	var valueProvidedInStorageClass bool
	if ssdStorageClassParameter != "" {
		valueProvidedInStorageClass = true
	}

	if valueProvidedInStorageClass {
		ssdValue, err = strconv.ParseBool(ssdStorageClassParameter)
		if err != nil {
			return ssdValue, err
		}
		slog.Debug("setting ssd value from storage class parameter", "value", ssdValue)
		return ssdValue, nil
	}

	// get the ssd value from the pool
	pool, err := client.GetPoolByName(ctx, poolName)
	if err != nil {
		slog.Error("determineSSDValue error", "error", err.Error())
		return ssdValue, err
	}
	slog.Debug("setting ssd value from pool", "value", pool.SsdEnabled)
	return pool.SsdEnabled, nil
}

func (sh StorageService) ValidateIPAddress(ip string, port int) (err error) {
	start := time.Now()

	ipAndPort := ip + ":" + strconv.Itoa(port)
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.Dial("tcp", ipAndPort)
	if conn != nil {
		if err := conn.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}
	elapsed := time.Since(start)

	if err != nil {
		slog.Error("error dialing IP address", "address", ipAndPort, "error", err.Error(), "elapsed", elapsed)
		return err
	}
	slog.Debug("IP address is reachable", "address", ipAndPort, "elapsed", elapsed)
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
		slog.Error(err.Error())
		return err
	}
	slog.Log(context.Background(), common.LevelTrace, "found path", "path", path)
	return nil
}

// Used for debugging. For given walk_path, log all files found within.
func DebugWalkDir(walkPath string) (err error) {
	slog.Log(context.Background(), common.LevelTrace, "walkPath", "path", walkPath)
	err = filepath.Walk(walkPath, DebugLogPath)
	if err != nil {
		slog.Error(err.Error())
		return err
	}
	return nil
}

func GetHostInfo(ctx context.Context, secrets map[string]string, client iboxapi.Client) (iboxInfo string) {
	sys, _ := client.GetSystem(ctx)
	var serialNumber int
	if sys != nil {
		serialNumber = sys.SerialNumber
	}
	return fmt.Sprintf(" - ibox %s (%d)", secrets[common.CredentialHostname], serialNumber)
}

func ValidatePublishContext(publishContext map[string]string) (hostID int, ports string, err error) {
	hostIDString := publishContext[HostIDPublishContext]
	hostID, err = strconv.Atoi(hostIDString)
	if err != nil {
		err := fmt.Errorf("hostID string '%s' is not valid host ID: %v", hostIDString, err)
		slog.Error(err.Error())
		return 0, "", status.Error(codes.Internal, err.Error())
	}

	if hostID < 1 {
		e := fmt.Errorf("hostID %d is not valid host ID", hostID)
		return 0, "", status.Error(codes.Internal, e.Error())
	}

	ports = publishContext[HostPortsPublishContext]

	return hostID, ports, nil
}

func HostCleanup(ctx context.Context, iboxClient iboxapi.Client, hostID int, hostName string) error {
	meta, err := iboxClient.GetMetadata(ctx, hostID)
	if err != nil {
		e := fmt.Errorf("hostCleanup: failed to get metadata for host ID %d. Error: %v", hostID, err)
		slog.Error(e.Error())
		return status.Error(codes.Internal, e.Error())
	}
	var createdByCSI bool
	for i := range meta {
		if meta[i].Key == common.CSICreatedHost {
			createdByCSI = true
		}
	}

	if createdByCSI {
		response, err := iboxClient.DeleteHost(ctx, hostID)
		if err != nil {
			re, ok := err.(*iboxapi.APIError)
			if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
				slog.Debug("hostCleanup: will not delete, host not found", "hostid", hostID, "response", response)
			} else {
				slog.Error("hostCleanup: failed to delete host with error", "error", err)
				return status.Error(codes.Internal, err.Error())
			}
		}
		slog.Debug("hostCleanup: deleted host on ibox because it was created by CSI host", "hostid", hostID, "hostname", hostName)
	} else {
		slog.Debug("hostCleanup: not deleting host because it was not created by CSI host", "hostid", hostID)
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
			slog.Debug("REMOVE_DOMAIN_NAME set to true, resulting in", "hostname", hostName, "short", shortName[0])
			hostName = shortName[0]
		}
	}
	return hostName, nil
}

func ValidateVolumeID(volumeIDString string) (volprotoconf api.VolumeProtocolConfig, err error) {
	if volumeIDString == "" {
		return volprotoconf, fmt.Errorf("volume Id string is empty, [%s]", volumeIDString)
	}
	volproto := strings.Split(volumeIDString, "$$")
	if len(volproto) != 2 {
		return volprotoconf, fmt.Errorf("volume Id and other details not found, [%s]", volumeIDString)
	}

	if volproto[0] == "" {
		return volprotoconf, fmt.Errorf("volume Id in volproto is empty, [%s]", volumeIDString)
	}

	if volproto[1] == "" {
		return volprotoconf, fmt.Errorf("volume storagetype in volproto is empty, [%s]", volumeIDString)
	}
	volprotoconf.StorageType = volproto[1]

	// treeq is a special formatting case
	if volprotoconf.StorageType == common.ProtocolTreeq {
		// example: volproto[0] == 2942184#20000
		tmp := strings.Split(volproto[0], "#")
		if len(tmp) != 2 {
			return volprotoconf, fmt.Errorf("treeq volume not correctly formatted %s, [%s]", volproto[0], volumeIDString)
		}
		volprotoconf.VolumeID, err = strconv.Atoi(tmp[0])
		if err != nil {
			return volprotoconf, err
		}
		slog.Debug("ValidateVolumeID treeq", "VolumeID", volprotoconf.VolumeID, "treeq id", tmp[1])

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
		slog.Error(e.Error())
		return volprotoconf, fmt.Errorf("volume id in volproto is not an integer, [%s]", volumeIDString)
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

func (cs *Commonservice) getStoragePoolNameFromID(ctx context.Context, poolID int) string {
	slog.Debug("called", "pool id", poolID)
	storagePoolName := cs.storagePoolIDName[poolID]
	if storagePoolName == "" {
		pool, err := cs.IboxAPI.GetPoolByID(ctx, poolID)
		if err == nil {
			storagePoolName = pool.Name
			cs.storagePoolIDName[poolID] = pool.Name
		} else {
			slog.Error("Could not find StoragePool", "pool id", poolID)
		}
	}
	return storagePoolName
}
func (cs *Commonservice) verifyAPIClient() error {
	slog.Log(context.Background(), common.LevelTrace, "verifying api client")
	c, err := cs.API.NewClient()
	if err != nil {
		slog.Error("api client is not working.")
		return errors.New("failed to create rest client")
	}
	cs.API = c
	slog.Log(context.Background(), common.LevelTrace, "api client is verified.")
	return nil
}
