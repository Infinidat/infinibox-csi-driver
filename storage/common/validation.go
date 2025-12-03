package common

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
)

// used to look up expected service for protocol
var protoToServiceMap = map[string]string{
	common.ProtocolNFS:   common.NetworkSpaceNFSService,
	common.ProtocolTreeq: common.NetworkSpaceNFSService,
	common.ProtocolISCSI: common.NetworkSpaceISCSIService,
	common.ProtocolNVME:  common.NetworkSpaceNVMEService,
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
		slog.Error(e.Error())
		return e
	}

	return nil
}

// validateProtocolToNetworkSpace - ensure specified protocol is valid for specified network space
func ValidateProtocolToNetworkSpace(ctx context.Context, protocol string, networkSpaces []string, api iboxapi.Client) error {
	if len(networkSpaces) == 0 {
		err := fmt.Errorf("no network spaces provided")
		slog.Error(err.Error())
		return err
	}

	for _, networkSpace := range networkSpaces {
		slog.Debug("validating", "ns", networkSpace, "protocol", protocol)
		nSpace, err := api.GetNetworkSpaceByName(ctx, networkSpace)
		if err != nil {
			// api call throws error
			slog.Error(err.Error())
			return err
		}
		if len(nSpace.Service) == 0 {
			// handle empty result - nSpace doesn't exist
			e := fmt.Errorf("ibox not configured with specified network space: '%s' Service is empty", networkSpace)
			slog.Error(e.Error())
			return e
		}
		if nSpace.Service != protoToServiceMap[protocol] {
			// handle invalid protocol/networkspace configuration
			e := fmt.Errorf("specified network space '%s' does not support %s protocol with %s service", networkSpace, protocol, nSpace.Service)
			slog.Error(e.Error())
			return e
		}
		slog.Debug("Network space supports protocol with service", "networkSpace", networkSpace, "protocol", protocol, "service", nSpace.Service)
	}

	return nil // returns here if all network spaces pass validation for protocol.
}
