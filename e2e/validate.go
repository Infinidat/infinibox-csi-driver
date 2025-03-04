package e2e

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"os"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ValidateEnv(testConfig *TestConfig) (err error) {

	// validate ibox pool
	poolToUse := os.Getenv(ENV_POOL)
	if poolToUse == "" {
		return fmt.Errorf("%s env var is not set and is required", ENV_POOL)
	}

	_, err = testConfig.ClientService.Iboxapi.GetPoolByName(poolToUse)
	if err != nil {
		return fmt.Errorf("error getting pool by name %s %w", poolToUse, err)
	}

	tmp := os.Getenv(ENV_STRESS_ITERATIONS)
	if tmp != "" {
		testConfig.StressIterations, err = strconv.Atoi(tmp)
		if err != nil {
			return fmt.Errorf("error parsing %s %s - error %s", ENV_STRESS_ITERATIONS, tmp, err.Error())
		}
	}
	useNFSV4 := os.Getenv(ENV_USE_NFS_V4)
	if useNFSV4 != "" {
		testConfig.UseNFSV4, err = strconv.ParseBool(useNFSV4)
		if err != nil {
			return fmt.Errorf("%s env var is set but is not a valid boolean", ENV_USE_NFS_V4)
		}
	}
	tmp = os.Getenv(ENV_STRESS_SLEEP_SECONDS)
	if tmp != "" {
		testConfig.StressSleepSeconds, err = strconv.Atoi(tmp)
		if err != nil {
			return fmt.Errorf("error parsing %s %s - error %s", ENV_STRESS_SLEEP_SECONDS, tmp, err.Error())
		}
	} else {
		testConfig.StressSleepSeconds = 15
	}

	// validate network space on the ibox
	protocol := os.Getenv(ENV_PROTOCOL)

	switch protocol {
	case common.PROTOCOL_FC, common.PROTOCOL_ISCSI, common.PROTOCOL_NFS, common.PROTOCOL_TREEQ, common.PROTOCOL_NVME:
		fmt.Printf("valid protocol found in env vars [%s]\n", protocol)
	default:
		return fmt.Errorf("%s env var value not recognized [%s], must be a valid protocol [%s,%s,%s,%s,%s]", ENV_PROTOCOL, protocol, common.PROTOCOL_FC, common.PROTOCOL_ISCSI, common.PROTOCOL_NFS, common.PROTOCOL_TREEQ, common.PROTOCOL_NVME)
	}

	if protocol != common.PROTOCOL_FC {
		networkSpaceToUse := os.Getenv(ENV_NETWORK_SPACE)
		if networkSpaceToUse == "" {
			return fmt.Errorf("%s env var is not set and is required", ENV_NETWORK_SPACE)
		}
		_, err = testConfig.ClientService.Iboxapi.GetNetworkSpaceByName(networkSpaceToUse)
		if err != nil {
			return fmt.Errorf("error getting network space by name %s %w", networkSpaceToUse, err)
		}
	}

	// validate network space 2 on the ibox if set
	networkSpace2ToUse := os.Getenv(ENV_NETWORK_SPACE2)
	if networkSpace2ToUse != "" {
		_, err = testConfig.ClientService.Iboxapi.GetNetworkSpaceByName(networkSpace2ToUse)
		if err != nil {
			return fmt.Errorf("error getting network space by name 2 %s %w", networkSpace2ToUse, err)
		}
	}

	// validate namespace on the kube
	namespaceToUse := os.Getenv(ENV_NAMESPACE)
	if namespaceToUse != "" {
		_, err := testConfig.ClientSet.CoreV1().Namespaces().Get(context.TODO(), namespaceToUse, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting namespace %s %w", namespaceToUse, err)
		}
	}

	// validate ibox secret on the kube
	iboxCredentialToUse := os.Getenv(ENV_IBOX_SECRET)
	if iboxCredentialToUse != "" {
		_, err := testConfig.ClientSet.CoreV1().Secrets(namespaceToUse).Get(context.TODO(), iboxCredentialToUse, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting secrets %s %w", namespaceToUse, err)
		}
	} else {
		return fmt.Errorf("%s not specified and is required", ENV_IBOX_SECRET)
	}

	// validate ibox secret2 on the kube if set
	iboxCredential2ToUse := os.Getenv(ENV_IBOX_SECRET2)
	if iboxCredential2ToUse != "" {
		_, err := testConfig.ClientSet.CoreV1().Secrets(namespaceToUse).Get(context.TODO(), iboxCredential2ToUse, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting secrets 2 %s %w", namespaceToUse, err)
		}
	}
	return nil
}
