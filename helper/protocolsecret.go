package helper

import (
	"fmt"
	"infinibox-csi-driver/api/clientgo"
	"infinibox-csi-driver/common"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func GetProtocolSecret() (protocolSecret map[string]string, found bool, err error) {

	const FN = "GetProtocolSecret"
	secretName := os.Getenv(common.ENV_VAR_PROTOCOL_SECRET)
	secretNamespace := os.Getenv(common.ENV_VAR_POD_NAMESPACE)

	if secretName == "" {
		return protocolSecret, false, nil
	}

	// the secret namespace is set in the installation, it should never
	// be blank, if so, it would be an error
	if secretName != "" && secretNamespace == "" {
		e := fmt.Errorf("%s - error - protocol secret namespace is blank - verify your StorageClass has the values set", FN)
		zlog.Error().Msg(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s - error %s - could not get kube client", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	protocolSecret, err = kubeClient.GetSecret(secretName, secretNamespace)
	if err != nil {
		// since secretName was specified, something has happened to
		// remove the secret, this would be an error condition
		e := fmt.Errorf("%s - error %s - could not get protocol secret", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	storageProtocol := protocolSecret[common.SC_STORAGE_PROTOCOL]
	// validate what the user entered for the protocol
	switch storageProtocol {
	case common.PROTOCOL_NFS, common.PROTOCOL_TREEQ:
		/**
		e := fmt.Errorf("%s - error - nfs and treeq are unsupported when using a protocol secret %s", FN, storageProtocol)
		zlog.Error().Msg(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
		*/
	case common.PROTOCOL_NVME:
	case common.PROTOCOL_FC:
	case common.PROTOCOL_ISCSI:
	case common.PROTOCOL_AUTO:
	default:
		e := fmt.Errorf("%s - error - unsupported protocol specified %s", FN, storageProtocol)
		zlog.Error().Msg(e.Error())
		return protocolSecret, false, status.Error(codes.InvalidArgument, e.Error())
	}

	zlog.Debug().Msgf("%s - secret protcol in use - %v", FN, protocolSecret)
	return protocolSecret, true, nil
}
