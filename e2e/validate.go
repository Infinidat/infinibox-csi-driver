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

	_, err = testConfig.ClientService.IboxAPI.GetPoolByName(poolToUse)
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
	case common.ProtocolFC, common.ProtocolISCSI, common.ProtocolNFS, common.ProtocolTreeq, common.ProtocolNVME:
		fmt.Printf("valid protocol found in env vars [%s]\n", protocol)
	default:
		return fmt.Errorf("%s env var value not recognized [%s], must be a valid protocol [%s,%s,%s,%s,%s]", ENV_PROTOCOL, protocol, common.ProtocolFC, common.ProtocolISCSI, common.ProtocolNFS, common.ProtocolTreeq, common.ProtocolNVME)
	}

	var nsEnvVar string

	switch protocol {
	case common.ProtocolFC:
	case common.ProtocolISCSI:
		nsEnvVar = ENV_ISCSI_NETWORK_SPACE
		testConfig.NetworkSpaceToUse = os.Getenv(ENV_ISCSI_NETWORK_SPACE)
	case common.ProtocolNFS, common.ProtocolTreeq:
		nsEnvVar = ENV_NAS_NETWORK_SPACE
		testConfig.NetworkSpaceToUse = os.Getenv(ENV_NAS_NETWORK_SPACE)
	case common.ProtocolNVME:
		nsEnvVar = ENV_NVME_NETWORK_SPACE
		testConfig.NetworkSpaceToUse = os.Getenv(ENV_NVME_NETWORK_SPACE)
	}

	// for backward compat only
	if protocol != common.ProtocolFC && testConfig.NetworkSpaceToUse == "" {
		nsEnvVar = ENV_NETWORK_SPACE
		testConfig.NetworkSpaceToUse = os.Getenv(ENV_NETWORK_SPACE)
	}

	if protocol != common.ProtocolFC {
		if testConfig.NetworkSpaceToUse == "" {
			return fmt.Errorf("%s env var is not set and is required", nsEnvVar)
		}
		_, err = testConfig.ClientService.IboxAPI.GetNetworkSpaceByName(testConfig.NetworkSpaceToUse)
		if err != nil {
			return fmt.Errorf("error getting network space by name %s %w", testConfig.NetworkSpaceToUse, err)
		}
	}

	// validate network space 2 on the ibox if set
	testConfig.NetworkSpaceToUse2 = os.Getenv(ENV_ISCSI_NETWORK_SPACE2)
	if testConfig.NetworkSpaceToUse2 == "" {
		// for backward compat only
		testConfig.NetworkSpaceToUse2 = os.Getenv(ENV_NETWORK_SPACE2)
	}
	if testConfig.NetworkSpaceToUse2 != "" {
		_, err = testConfig.ClientService.IboxAPI.GetNetworkSpaceByName(testConfig.NetworkSpaceToUse2)
		if err != nil {
			return fmt.Errorf("error getting network space by name 2 %s %w", testConfig.NetworkSpaceToUse2, err)
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
