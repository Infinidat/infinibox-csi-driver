package e2e

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ValidateEnv(testConfig *TestConfig) (err error) {

	// validate ibox pool
	poolToUse := os.Getenv("_E2E_POOL")
	if poolToUse == "" {
		return fmt.Errorf("_E2E_POOL env var is not set and is required")
	}
	_, err = testConfig.ClientService.GetStoragePoolIDByName(poolToUse)
	if err != nil {
		return err
	}

	// validate network space on the ibox
	protocol := os.Getenv("_E2E_PROTOCOL")
	if protocol != common.PROTOCOL_FC {
		networkSpaceToUse := os.Getenv("_E2E_NETWORK_SPACE")
		if networkSpaceToUse == "" {
			return fmt.Errorf("_E2E_NETWORK_SPACE env var is not set and is required")
		}
		_, err = testConfig.ClientService.GetNetworkSpaceByName(networkSpaceToUse)
		if err != nil {
			return err
		}
	}

	// validate network space 2 on the ibox if set
	networkSpace2ToUse := os.Getenv("_E2E_NETWORK_SPACE2")
	if networkSpace2ToUse != "" {
		_, err = testConfig.ClientService.GetNetworkSpaceByName(networkSpace2ToUse)
		if err != nil {
			return err
		}
	}

	// validate namespace on the kube
	namespaceToUse := os.Getenv("_E2E_NAMESPACE")
	if namespaceToUse != "" {
		_, err := testConfig.ClientSet.CoreV1().Namespaces().Get(context.TODO(), namespaceToUse, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}

	// validate ibox secret on the kube
	iboxCredentialToUse := os.Getenv("_E2E_IBOX_SECRET")
	if iboxCredentialToUse != "" {
		_, err := testConfig.ClientSet.CoreV1().Secrets(namespaceToUse).Get(context.TODO(), iboxCredentialToUse, metav1.GetOptions{})
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("_E2E_IBOX_SECRET not specified and is required")
	}

	// validate ibox secret2 on the kube if set
	iboxCredential2ToUse := os.Getenv("_E2E_IBOX_SECRET2")
	if iboxCredential2ToUse != "" {
		_, err := testConfig.ClientSet.CoreV1().Secrets(namespaceToUse).Get(context.TODO(), iboxCredential2ToUse, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}
