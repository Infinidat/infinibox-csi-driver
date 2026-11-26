//go:build unit

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

package luks

import (
	"context"
	"errors"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func createVolumeParams(extra map[string]string) map[string]string {
	params := map[string]string{
		common.StorageClassStorageProtocol: common.ProtocolISCSI,
		common.CSIPVCName:                  "data",
		common.CSIPVCNamespace:             "tenant-a",
		common.CSIPVName:                   "csi-0123456789",
	}
	for k, v := range extra {
		params[k] = v
	}
	return params
}

func TestIsEncrypted(t *testing.T) {
	for value, want := range map[string]bool{"": false, "false": false, "true": true, "True": true, "1": true} {
		got, err := IsEncrypted(map[string]string{common.StorageClassEncrypted: value})
		require.NoError(t, err, value)
		assert.Equal(t, want, got, value)
	}
	_, err := IsEncrypted(map[string]string{common.StorageClassEncrypted: "maybe"})
	assert.Error(t, err)
}

func TestVolumeContextFromParameters(t *testing.T) {
	t.Run("not encrypted", func(t *testing.T) {
		vc, err := VolumeContextFromParameters(common.ProtocolISCSI, createVolumeParams(nil))
		require.NoError(t, err)
		assert.Nil(t, vc)
	})

	t.Run("templates are resolved", func(t *testing.T) {
		vc, err := VolumeContextFromParameters(common.ProtocolISCSI, createVolumeParams(map[string]string{
			common.StorageClassEncrypted:                 "true",
			common.StorageClassEncryptionSecretName:      "${pvc.name}-luks",
			common.StorageClassEncryptionSecretNamespace: "${pvc.namespace}",
		}))
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			common.StorageClassEncrypted:                 "true",
			common.StorageClassEncryptionSecretName:      "data-luks",
			common.StorageClassEncryptionSecretNamespace: "tenant-a",
		}, vc)
	})

	t.Run("fixed names and pv name", func(t *testing.T) {
		vc, err := VolumeContextFromParameters(common.ProtocolFC, createVolumeParams(map[string]string{
			common.StorageClassEncrypted:                 "true",
			common.StorageClassEncryptionSecretName:      "luks-${pv.name}",
			common.StorageClassEncryptionSecretNamespace: "infinidat-csi",
		}))
		require.NoError(t, err)
		assert.Equal(t, "luks-csi-0123456789", vc[common.StorageClassEncryptionSecretName])
		assert.Equal(t, "infinidat-csi", vc[common.StorageClassEncryptionSecretNamespace])
	})

	for name, tc := range map[string]struct {
		protocol string
		params   map[string]string
		want     string
	}{
		"nfs": {common.ProtocolNFS, map[string]string{common.StorageClassEncrypted: "true",
			common.StorageClassEncryptionSecretName: "s", common.StorageClassEncryptionSecretNamespace: "ns"}, "block protocols"},
		"treeq": {common.ProtocolTreeq, map[string]string{common.StorageClassEncrypted: "true",
			common.StorageClassEncryptionSecretName: "s", common.StorageClassEncryptionSecretNamespace: "ns"}, "block protocols"},
		"missing secret name": {common.ProtocolNVME, map[string]string{common.StorageClassEncrypted: "true",
			common.StorageClassEncryptionSecretNamespace: "ns"}, "are required"},
		"unknown template": {common.ProtocolISCSI, map[string]string{common.StorageClassEncrypted: "true",
			common.StorageClassEncryptionSecretName: "${pvc.annotations['x']}", common.StorageClassEncryptionSecretNamespace: "ns"}, "unknown template"},
		"invalid name": {common.ProtocolISCSI, map[string]string{common.StorageClassEncrypted: "true",
			common.StorageClassEncryptionSecretName: "Not_Valid", common.StorageClassEncryptionSecretNamespace: "ns"}, "not a valid Secret name"},
		"invalid boolean": {common.ProtocolISCSI, map[string]string{common.StorageClassEncrypted: "yes please"}, "true or false"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := VolumeContextFromParameters(tc.protocol, createVolumeParams(tc.params))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("templates need extra-create-metadata", func(t *testing.T) {
		params := map[string]string{
			common.StorageClassEncrypted:                 "true",
			common.StorageClassEncryptionSecretName:      "${pvc.name}",
			common.StorageClassEncryptionSecretNamespace: "${pvc.namespace}",
		}
		_, err := VolumeContextFromParameters(common.ProtocolISCSI, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--extra-create-metadata")
	})
}

func TestSecretRefFromVolumeContext(t *testing.T) {
	_, encrypted, err := SecretRefFromVolumeContext(map[string]string{common.StorageClassStorageProtocol: common.ProtocolISCSI})
	require.NoError(t, err)
	assert.False(t, encrypted)

	ref, encrypted, err := SecretRefFromVolumeContext(map[string]string{
		common.StorageClassEncrypted:                 "true",
		common.StorageClassEncryptionSecretName:      "data-luks",
		common.StorageClassEncryptionSecretNamespace: "tenant-a",
	})
	require.NoError(t, err)
	assert.True(t, encrypted)
	assert.Equal(t, SecretRef{Name: "data-luks", Namespace: "tenant-a"}, ref)

	// a volume with a Secret reference that cannot be used must fail, not be mounted in clear
	for _, vc := range []map[string]string{
		{common.StorageClassEncrypted: "true", common.StorageClassEncryptionSecretName: "data-luks"},
		{common.StorageClassEncrypted: "true", common.StorageClassEncryptionSecretNamespace: "tenant-a"},
		{common.StorageClassEncrypted: "true", common.StorageClassEncryptionSecretName: "${pvc.name}", common.StorageClassEncryptionSecretNamespace: "ns"},
	} {
		_, encrypted, err := SecretRefFromVolumeContext(vc)
		assert.True(t, encrypted, vc)
		assert.Error(t, err, vc)
	}
	_, _, err = SecretRefFromVolumeContext(map[string]string{common.StorageClassEncrypted: "yes",
		common.StorageClassEncryptionSecretName: "data-luks", common.StorageClassEncryptionSecretNamespace: "tenant-a"})
	assert.Error(t, err)

	// StorageClass with encrypted=false: the Secret parameters are copied as they are, templates included
	_, encrypted, err = SecretRefFromVolumeContext(map[string]string{common.StorageClassEncrypted: "false",
		common.StorageClassEncryptionSecretName: "${pvc.name}-luks", common.StorageClassEncryptionSecretNamespace: "${pvc.namespace}"})
	require.NoError(t, err)
	assert.False(t, encrypted)
}

// Volumes provisioned before host-side encryption existed have all their StorageClass
// parameters in the volume context, including an "encrypted" parameter that the driver
// ignored: they must keep being mounted exactly as before.
func TestSecretRefFromVolumeContextLegacyVolumes(t *testing.T) {
	for _, value := range []string{"true", "false", "yes", "TRUE", "aws-kms"} {
		vc := map[string]string{
			common.StorageClassStorageProtocol: common.ProtocolISCSI,
			common.StorageClassEncrypted:       value,
		}
		_, encrypted, err := SecretRefFromVolumeContext(vc)
		require.NoError(t, err, value)
		assert.False(t, encrypted, value)
		assert.NoError(t, CheckVolumeContext(common.ProtocolNFS, vc), value)
		assert.NoError(t, CheckVolumeContext(common.ProtocolISCSI, vc), value)
	}
}

func TestCheckVolumeContext(t *testing.T) {
	encrypted := map[string]string{common.StorageClassEncrypted: "true",
		common.StorageClassEncryptionSecretName: "data-luks", common.StorageClassEncryptionSecretNamespace: "tenant-a"}
	assert.NoError(t, CheckVolumeContext(common.ProtocolNFS, map[string]string{}))
	assert.NoError(t, CheckVolumeContext(common.ProtocolNVME, encrypted))
	assert.Error(t, CheckVolumeContext(common.ProtocolNFS, encrypted))
	assert.Error(t, CheckVolumeContext(common.ProtocolTreeq, encrypted))
	assert.Error(t, CheckVolumeContext(common.ProtocolISCSI, map[string]string{common.StorageClassEncrypted: "true",
		common.StorageClassEncryptionSecretName: "data-luks"}))
}

func TestGetPassphrase(t *testing.T) {
	ref := SecretRef{Name: "data-luks", Namespace: "tenant-a"}
	saved := GetSecretFunc
	t.Cleanup(func() { GetSecretFunc = saved })

	secret := map[string]string{}
	var secretErr error
	GetSecretFunc = func(_ context.Context, name, namespace string) (map[string]string, error) {
		assert.Equal(t, ref.Name, name)
		assert.Equal(t, ref.Namespace, namespace)
		return secret, secretErr
	}

	secret[SecretPassphraseKey] = "correct horse battery staple\n"
	got, err := GetPassphrase(t.Context(), ref)
	require.NoError(t, err)
	assert.Equal(t, []byte("correct horse battery staple\n"), got, "the passphrase is used byte for byte")

	secret[SecretPassphraseKey] = ""
	_, err = GetPassphrase(t.Context(), ref)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	delete(secret, SecretPassphraseKey)
	_, err = GetPassphrase(t.Context(), ref)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	secretErr = errors.New("secrets \"data-luks\" not found")
	_, err = GetPassphrase(t.Context(), ref)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, err.Error(), "tenant-a/data-luks")
}
