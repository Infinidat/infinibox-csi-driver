/*
Copyright 2026 Infinidat
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

package common

import (
	"context"
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
		return common.Errorf("invalid StorageClass parameters provided: %s", badParamsMap)
	}

	return nil
}

// validateProtocolToNetworkSpace - ensure specified protocol is valid for specified network space
func ValidateProtocolToNetworkSpace(ctx context.Context, protocol string, networkSpaces []string, api iboxapi.Client) error {
	if len(networkSpaces) == 0 {
		return common.Errorf("no network spaces provided")
	}

	for _, networkSpace := range networkSpaces {
		slog.Debug("validating", "ns", networkSpace, "protocol", protocol)
		nSpace, err := api.GetNetworkSpaceByName(ctx, networkSpace)
		if err != nil {
			return err
		}
		if len(nSpace.Service) == 0 {
			return common.Errorf("ibox not configured with specified network space: '%s' Service is empty", networkSpace)
		}
		if nSpace.Service != protoToServiceMap[protocol] {
			return common.Errorf("specified network space '%s' does not support %s protocol with %s service", networkSpace, protocol, nSpace.Service)
		}
		slog.Debug("Network space supports protocol with service", "networkSpace", networkSpace, "protocol", protocol, "service", nSpace.Service)
	}

	return nil // returns here if all network spaces pass validation for protocol.
}
