package storage

import (
	"errors"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	StoragePoolKey = "storagepool"

	MinVolumeSize = 1 * bytesofGiB

	// VolSizeMultipleGiB = 1

	bytesofKiB = 1024

	kiBytesofGiB = 1024 * 1024

	bytesofGiB = kiBytesofGiB * bytesofKiB
)

func verifyVolumeSize(caprange *csi.CapacityRange) (int64, error) {
	requiredVolSize := int64(caprange.GetRequiredBytes())
	allowedMaxVolSize := int64(caprange.GetLimitBytes())
	if requiredVolSize < 0 || allowedMaxVolSize < 0 {
		return 0, errors.New("Not valid volume size.")
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

// Copies the interesting parameters to the output map.
func copyRequestParameters(parameters, out map[string]string) {
	for _, str := range mergedParameters {
		if parameters[str] != "" {
			out[str] = parameters[str]
		}
	}
}
