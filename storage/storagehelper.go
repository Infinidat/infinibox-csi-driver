package storage

import (
	"errors"
	"infinibox-csi-driver/api"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
)

const (
	//StoragePoolKey : pool to be used
	StoragePoolKey = "storagepool"

	//MinVolumeSize : volume will be created with this size if requested volume size is less than this values
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

func mergeStringMaps(base map[string]string, additional map[string]string) map[string]string {
	result := make(map[string]string)
	if base != nil {
		for k, v := range base {
			result[k] = v
		}
	}
	if additional != nil {
		for k, v := range additional {
			result[k] = v
		}
	}
	return result

}

func copyRequestParameters(parameters, out map[string]string) {
	for key, val := range parameters {
		if val != "" {
			out[key] = val
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
