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

package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ProtocolSecretISCSINetworkSpace    = "iscsi.network_space"
	ProtocolSecretISCSIUseCHAP         = "iscsi.useCHAP"
	ProtocolSecretNVMENetworkSpace     = "nvme.network_space"
	ProtocolSecretNFSNetworkSpace      = "nfs.network_space"
	ProtocolSecretNFSExportPermissions = "nfs.nfs_export_permissions"
)

func GetProtocolSecret(ctx context.Context) (protocolSecret map[string]string, found bool, err error) {
	secretName := os.Getenv(common.EnvVarProtocolSecret)
	secretNamespace := os.Getenv(common.EnvVarPodNamespace)

	if secretName == "" {
		return protocolSecret, false, nil
	}

	// the secret namespace is set in the installation, it should never
	// be blank, if so, it would be an error
	if secretName != "" && secretNamespace == "" {
		e := fmt.Errorf("error - protocol secret namespace is blank - verify your StorageClass has the values set")
		slog.Error(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("error %s - could not get kube client", err.Error())
		slog.Error(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	protocolSecret, err = kubeClient.GetSecret(ctx, secretName, secretNamespace)
	if err != nil {
		// since secretName was specified, something has happened to
		// remove the secret, this would be an error condition
		e := fmt.Errorf("error %s - could not get protocol secret", err.Error())
		slog.Error(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	storageProtocol := protocolSecret[common.StorageClassStorageProtocol]
	// validate what the user entered for the protocol
	switch storageProtocol {
	case "":
	case common.ProtocolNFS, common.ProtocolTreeq:
	case common.ProtocolNVME:
	case common.ProtocolFC:
	case common.ProtocolISCSI:
	case common.ProtocolAuto:
	default:
		e := fmt.Errorf("error - unsupported protocol specified %s", storageProtocol)
		slog.Error(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	slog.Debug("secret protocol in use", "secret", protocolSecret)
	return protocolSecret, true, nil
}
