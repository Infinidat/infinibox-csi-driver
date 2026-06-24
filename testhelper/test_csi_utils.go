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

package testhelper

import (
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func GetSecret() map[string]string {
	return map[string]string{
		"username": "admin",
		"password": "123456",
		"hostname": "https://172.17.35.61/",
	}
}

func GetHostMetadata() (results []iboxapi.GetMetadataResult) {
	metadata := iboxapi.GetMetadataResult{
		Key:   common.CSICreatedHost,
		Value: "true",
	}
	results = append(results, metadata)
	return results
}

// TODO: below only generates a MountVolume request, not a BlockVolume request. We should test both. CSIC-342
func GetCreateVolumeRequest(name string, parameterMap map[string]string, sourceVolID string) *csi.CreateVolumeRequest {
	var volContentSrc *csi.VolumeContentSource
	if sourceVolID != "" {
		volContentSrc = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: sourceVolID,
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
