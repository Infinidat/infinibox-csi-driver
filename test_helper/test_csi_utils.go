package test_helper

import (
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func GetSecret() map[string]string {
	return map[string]string{
		"username": "admin",
		"password": "123456",
		"hostname": "https://172.17.35.61/",
	}
}

func GetHostMetadata() (results []api.MetadataResult) {
	metadata := api.MetadataResult{
		Key:   common.CSI_CREATED_HOST,
		Value: "true",
	}
	results = append(results, metadata)
	return results
}

// TODO: below only generates a MountVolume request, not a BlockVolume request. We should test both. CSIC-342
func GetCreateVolumeRequest(name string, parameterMap map[string]string, sourceVolId string) *csi.CreateVolumeRequest {
	var volContentSrc *csi.VolumeContentSource
	if sourceVolId != "" {
		volContentSrc = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: sourceVolId,
				},
			},
		}
	}

	return &csi.CreateVolumeRequest{
		Name:                name,
		CapacityRange:       &csi.CapacityRange{RequiredBytes: common.BytesInOneGibibyte},
		Parameters:          parameterMap,
		Secrets:             GetSecret(),
		VolumeContentSource: volContentSrc,
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{}, // TODO: should specify fstype here in line with spec
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	}
}
