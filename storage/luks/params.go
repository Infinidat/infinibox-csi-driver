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
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/validation"
)

// SecretPassphraseKey is the key of the passphrase in the encryption Secret.
const SecretPassphraseKey = "passphrase"

// SecretRef points to the Secret that holds the passphrase of one volume.
type SecretRef struct {
	Name      string
	Namespace string
}

func (r SecretRef) String() string {
	return r.Namespace + "/" + r.Name
}

// Validate checks that the reference is a valid Secret name and namespace, so that
// an unresolved template such as ${pvc.name} is never used as a name.
func (r SecretRef) Validate() error {
	if r.Name == "" || r.Namespace == "" {
		return common.Errorf("%s and %s are required when %s is true",
			common.StorageClassEncryptionSecretName, common.StorageClassEncryptionSecretNamespace, common.StorageClassEncrypted)
	}
	if errs := validation.IsDNS1123Subdomain(r.Name); len(errs) > 0 {
		return common.Errorf("%s %q is not a valid Secret name: %s", common.StorageClassEncryptionSecretName, r.Name, strings.Join(errs, ", "))
	}
	if errs := validation.IsDNS1123Label(r.Namespace); len(errs) > 0 {
		return common.Errorf("%s %q is not a valid namespace: %s", common.StorageClassEncryptionSecretNamespace, r.Namespace, strings.Join(errs, ", "))
	}
	return nil
}

// IsEncrypted reports whether StorageClass parameters (or a volume context) ask for encryption.
func IsEncrypted(params map[string]string) (bool, error) {
	value := params[common.StorageClassEncrypted]
	if value == "" {
		return false, nil
	}
	encrypted, err := strconv.ParseBool(value)
	if err != nil {
		return false, common.Errorf("%s must be true or false, got %q", common.StorageClassEncrypted, value)
	}
	return encrypted, nil
}

// VolumeContextFromParameters validates the encryption parameters of a CreateVolume request
// and returns the entries to set in the volume context of the new volume, with the
// templates of the Secret reference resolved. It returns nil for a volume that is not encrypted.
//
// The templates are those of the CSI external-provisioner: ${pvc.name}, ${pvc.namespace}
// and ${pv.name}. They are resolved from the parameters that the provisioner adds with
// --extra-create-metadata.
func VolumeContextFromParameters(storageProtocol string, params map[string]string) (map[string]string, error) {
	encrypted, err := IsEncrypted(params)
	if err != nil || !encrypted {
		return nil, err
	}
	if !isBlockProtocol(storageProtocol) {
		return nil, protocolError(storageProtocol)
	}

	name, err := resolveTemplate(params[common.StorageClassEncryptionSecretName], params)
	if err != nil {
		return nil, common.Errorf("%s: %w", common.StorageClassEncryptionSecretName, err)
	}
	namespace, err := resolveTemplate(params[common.StorageClassEncryptionSecretNamespace], params)
	if err != nil {
		return nil, common.Errorf("%s: %w", common.StorageClassEncryptionSecretNamespace, err)
	}
	ref := SecretRef{Name: name, Namespace: namespace}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return map[string]string{
		common.StorageClassEncrypted:                 "true",
		common.StorageClassEncryptionSecretName:      ref.Name,
		common.StorageClassEncryptionSecretNamespace: ref.Namespace,
	}, nil
}

// SecretRefFromVolumeContext returns the Secret reference of an encrypted volume, and
// false for a volume that is not encrypted.
//
// A volume is encrypted when its volume context has encrypted=true and a Secret reference,
// which CreateVolume always writes for encrypted volumes. The driver copies all the
// StorageClass parameters into the volume context, so a volume provisioned before
// encryption existed can carry an "encrypted" parameter that the driver ignored: without
// a Secret reference it keeps being mounted as before. A volume with a Secret reference
// that is not valid is an error: it is never mounted without encryption.
func SecretRefFromVolumeContext(volumeContext map[string]string) (SecretRef, bool, error) {
	ref := SecretRef{
		Name:      volumeContext[common.StorageClassEncryptionSecretName],
		Namespace: volumeContext[common.StorageClassEncryptionSecretNamespace],
	}
	if ref.Name == "" && ref.Namespace == "" {
		if value := volumeContext[common.StorageClassEncrypted]; value != "" {
			slog.Warn("the volume context has an encrypted parameter but no encryption Secret: "+
				"the volume was provisioned without host-side encryption and is mounted as before",
				common.StorageClassEncrypted, value)
		}
		return SecretRef{}, false, nil
	}
	encrypted, err := IsEncrypted(volumeContext)
	if err != nil || !encrypted {
		return SecretRef{}, false, err
	}
	if err := ref.Validate(); err != nil {
		return SecretRef{}, true, err
	}
	return ref, true, nil
}

// CheckVolumeContext fails for a volume that is encrypted (see SecretRefFromVolumeContext)
// with a protocol that does not support encryption, or whose Secret reference is not valid.
func CheckVolumeContext(storageProtocol string, volumeContext map[string]string) error {
	_, encrypted, err := SecretRefFromVolumeContext(volumeContext)
	if err != nil {
		return err
	}
	if encrypted && !isBlockProtocol(storageProtocol) {
		return protocolError(storageProtocol)
	}
	return nil
}

// only block volumes can be encrypted
func isBlockProtocol(storageProtocol string) bool {
	switch storageProtocol {
	case common.ProtocolFC, common.ProtocolISCSI, common.ProtocolNVME:
		return true
	}
	return false
}

func protocolError(storageProtocol string) error {
	return common.Errorf("%s is supported only for block protocols (%s, %s, %s), not for %q",
		common.StorageClassEncrypted, common.ProtocolFC, common.ProtocolISCSI, common.ProtocolNVME, storageProtocol)
}

var templateRe = regexp.MustCompile(`\$\{[^}]*\}`)

func resolveTemplate(value string, params map[string]string) (string, error) {
	vars := map[string]string{
		"${pvc.name}":      params[common.CSIPVCName],
		"${pvc.namespace}": params[common.CSIPVCNamespace],
		"${pv.name}":       params[common.CSIPVName],
	}
	var resolveErr error
	resolved := templateRe.ReplaceAllStringFunc(value, func(token string) string {
		v, known := vars[token]
		switch {
		case !known:
			resolveErr = fmt.Errorf("unknown template %s in %q, only ${pvc.name}, ${pvc.namespace} and ${pv.name} are supported", token, value)
		case v == "":
			resolveErr = fmt.Errorf("cannot resolve %s in %q: the csi-provisioner sidecar must run with --extra-create-metadata", token, value)
		}
		return v
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	return resolved, nil
}

// GetSecretFunc reads a Secret. The node plugin already has the RBAC to read Secrets.
// Tests replace it.
var GetSecretFunc = func(ctx context.Context, name, namespace string) (map[string]string, error) {
	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		return nil, common.Errorf("could not get kube client: %w", err)
	}
	return kubeClient.GetSecret(ctx, name, namespace)
}

// GetPassphrase reads the passphrase of an encrypted volume from its Secret. The value is
// used as is, byte for byte, like a cryptsetup key file: a trailing newline is part of it.
func GetPassphrase(ctx context.Context, ref SecretRef) ([]byte, error) {
	data, err := GetSecretFunc(ctx, ref.Name, ref.Namespace)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, common.Errorf("cannot read the encryption Secret %s: %w", ref, err).Error())
	}
	passphrase := data[SecretPassphraseKey]
	if passphrase == "" {
		return nil, status.Error(codes.FailedPrecondition, common.Errorf("the encryption Secret %s has no %q key or it is empty", ref, SecretPassphraseKey).Error())
	}
	if strings.HasSuffix(passphrase, "\n") {
		slog.Warn("the encryption passphrase ends with a newline, which is part of the passphrase", "secret", ref.String())
	}
	return []byte(passphrase), nil
}
