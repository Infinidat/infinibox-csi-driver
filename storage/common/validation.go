package common

import (
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	"regexp"
)

// used to look up expected service for protocol
var protoToServiceMap = map[string]string{
	common.PROTOCOL_NFS:   common.NS_NFS_SVC,
	common.PROTOCOL_TREEQ: common.NS_NFS_SVC,
	common.PROTOCOL_ISCSI: common.NS_ISCSI_SVC,
	common.PROTOCOL_NVME:  common.NS_NVME_SVC,
}

func ValidateRequiredOptionalSCParameters(requiredStorageClassParams, optionalSCParameters map[string]string, providedStorageClassParams map[string]string) error {
	// Loop through and check required parameters only, consciously ignore parameters that aren't required
	badParamsMap := make(map[string]string)
	for param, requiredRegex := range requiredStorageClassParams {
		if paramValue, ok := providedStorageClassParams[param]; ok {
			if matched, _ := regexp.MatchString(requiredRegex, paramValue); !matched {
				badParamsMap[param] = "required input parameter " + paramValue + " didn't match expected pattern " + requiredRegex
			}
		} else {
			badParamsMap[param] = "parameter required but not provided"
		}
	}

	for param, requiredRegex := range optionalSCParameters {
		if paramValue, ok := providedStorageClassParams[param]; ok {
			if matched, _ := regexp.MatchString(requiredRegex, paramValue); !matched {
				badParamsMap[param] = "Optional input parameter " + paramValue + " didn't match expected pattern " + requiredRegex
			}
		}
	}

	if len(badParamsMap) > 0 {
		e := fmt.Errorf("invalid StorageClass parameters provided: %s", badParamsMap)
		zlog.Err(e)
		return e
	}

	return nil
}

// validateProtocolToNetworkSpace - ensure specified protocol is valid for specified network space
func ValidateProtocolToNetworkSpace(protocol string, networkSpaces []string, api iboxapi.Client) error {
	if len(networkSpaces) == 0 {
		err := fmt.Errorf("no network spaces provided")
		zlog.Err(err)
		return err
	}

	for _, networkSpace := range networkSpaces {
		zlog.Debug().Msgf("validating ns=%s protocol=%s", networkSpace, protocol)
		nSpace, err := api.GetNetworkSpaceByName(networkSpace)
		if err != nil {
			// api call throws error
			zlog.Err(err)
			return err
		}
		if len(nSpace.Service) == 0 {
			// handle empty result - nSpace doesn't exist
			e := fmt.Errorf("ibox not configured with specified network space: '%s' Service is empty", networkSpace)
			zlog.Err(e)
			return e
		}
		if nSpace.Service != protoToServiceMap[protocol] {
			// handle invalid protocol/networkspace configuration
			e := fmt.Errorf("specified network space '%s' does not support %s protocol with %s service", networkSpace, protocol, nSpace.Service)
			zlog.Err(e)
			return e
		}
		zlog.Debug().Msgf("Network space %s supports %s protocol with %s service", networkSpace, protocol, nSpace.Service)
	}

	return nil // returns here if all network spaces pass validation for protocol.
}
