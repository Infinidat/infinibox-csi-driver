/*Copyright 2020 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"strconv"
	"strings"

	log "infinibox-csi-driver/helper/logger"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog"
)

const (
	// StoragePoolKey : pool to be used
	StoragePoolKey = "pool_name"

	// MinVolumeSize : volume will be created with this size if requested volume size is less than this values
	MinVolumeSize = 1 * bytesofGiB

	bytesofKiB = 1024

	kiBytesofGiB = 1024 * 1024

	bytesofGiB = kiBytesofGiB * bytesofKiB
)

func verifyVolumeSize(caprange *csi.CapacityRange) (int64, error) {
	requiredVolSize := int64(caprange.GetRequiredBytes())
	allowedMaxVolSize := int64(caprange.GetLimitBytes())
	if requiredVolSize < 0 || allowedMaxVolSize < 0 {
		return 0, errors.New("not valid volume size")
	}

	if requiredVolSize == 0 {
		requiredVolSize = MinVolumeSize
	}

	var (
		sizeinGB   int64
		sizeinByte int64
	)

	sizeinGB = requiredVolSize / bytesofGiB
	if sizeinGB == 0 {
		log.Warn("Volumen Minimum capacity should be greater 1 GB")
		sizeinGB = 1
	}

	sizeinByte = sizeinGB * bytesofGiB
	if allowedMaxVolSize != 0 {
		if sizeinByte > allowedMaxVolSize {
			return 0, errors.New("volume size is out of allowed limit")
		}
	}

	return sizeinByte, nil
}

func validateParametersFC(storageClassParams map[string]string) error {
	reqParams := []string{
		"fstype",
		"pool_name",
		"provision_type",
		"storage_protocol",
		"ssd_enabled",
		"max_vols_per_host",
	}
	if len(reqParams) != len(storageClassParams) {
		log.Error("Mismatch in provided parameters and required params")
		return errors.New("Mismatch in provided parameters and required params")
	}
	for _, param := range reqParams {
		if storageClassParams[param] == "" {
			log.Errorf("Invalid value %s for required parameter %s", storageClassParams[param], param)
			return fmt.Errorf("Invalid value %s for required parameter %s", storageClassParams[param], param)
		}
	}
	return nil
}

func validateParametersiSCSI(storageClassParams map[string]string) error {
	reqParams := []string{
		"useCHAP",
		"fstype",
		"pool_name",
		"network_space",
		"provision_type",
		"storage_protocol",
		"ssd_enabled",
		"max_vols_per_host",
	}
	if len(reqParams) != len(storageClassParams) {
		log.Error("Mismatch in provided parameters and required params")
		return errors.New("Mismatch in provided parameters and required params")
	}
	for _, param := range reqParams {
		if storageClassParams[param] == "" {
			log.Errorf("Invalid value %s for required parameter %s", storageClassParams[param], param)
			return fmt.Errorf("Invalid value %s for required parameter %s", storageClassParams[param], param)
		}
	}
	return nil
}

func copyRequestParameters(parameters, out map[string]string) {
	for key, val := range parameters {
		if val != "" {
			out[key] = val
			klog.V(2).Infof("%s: %s", key, val)
		} else {
			klog.V(2).Infof("%s: empty", key)
		}
	}
}

func validateStorageType(str string) (volprotoconf api.VolumeProtocolConfig, err error) {
	volproto := strings.Split(str, "$$")
	if len(volproto) != 2 {
		return volprotoconf, errors.New("volume Id and other details not found")
	}
	volprotoconf.VolumeID = volproto[0]
	volprotoconf.StorageType = volproto[1]
	return volprotoconf, nil
}

func getPermissionMaps(permission string) ([]map[string]interface{}, error) {
	permissionFixed := strings.Replace(permission, "'", "\"", -1)
	var permissionsMapArray []map[string]interface{}
	err := json.Unmarshal([]byte(permissionFixed), &permissionsMapArray)
	if err != nil {
		klog.Errorf("invalid nfs_export_permissions format %v", err)
	}

	for _, pass := range permissionsMapArray {
		no_root_squash_str, ok := pass["no_root_squash"].(string)
		if ok {
			rootsq, err := strconv.ParseBool(no_root_squash_str)
			if err != nil {
				klog.V(4).Infof("failed to cast no_root_squash value in export permission - setting default value 'true'")
				rootsq = true
			}
			pass["no_root_squash"] = rootsq
		}
	}
	return permissionsMapArray, err
}
