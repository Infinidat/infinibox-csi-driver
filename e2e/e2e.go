package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"infinibox-csi-driver/api"
	"infinibox-csi-driver/log"

	pb "github.com/container-storage-interface/spec/lib/go/csi"
	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	groupsnapshotv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/typed/volumegroupsnapshot/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	SOCAT_SERVICE_PORT                      = "30007"
	ENV_IBOX_HOSTNAME                       = "_E2E_IBOX_HOSTNAME"
	ENV_IBOX_USERNAME                       = "_E2E_IBOX_USERNAME"
	ENV_IBOX_PASSWORD                       = "_E2E_IBOX_PASSWORD"
	ENV_PROTOCOL                            = "_E2E_PROTOCOL"
	ENV_POOL                                = "_E2E_POOL"
	ENV_NETWORK_SPACE                       = "_E2E_NETWORK_SPACE" // for backward compat, don't use for new stuff
	ENV_NAS_NETWORK_SPACE                   = "_E2E_NAS_NETWORK_SPACE"
	ENV_NVME_NETWORK_SPACE                  = "_E2E_NVME_NETWORK_SPACE"
	ENV_ISCSI_NETWORK_SPACE                 = "_E2E_ISCSI_NETWORK_SPACE"
	ENV_NETWORK_SPACE2                      = "_E2E_NETWORK_SPACE2" // for backward compat, don't use for new stuff
	ENV_ISCSI_NETWORK_SPACE2                = "_E2E_ISCSI_NETWORK_SPACE2"
	ENV_IBOX_SECRET                         = "_E2E_IBOX_SECRET"
	ENV_IBOX_SECRET2                        = "_E2E_IBOX_SECRET2"
	ENV_NAMESPACE                           = "_E2E_NAMESPACE"
	ENV_CLEANUP                             = "_E2E_CLEANUP"
	ENV_TEST_IMAGE                          = "_E2E_TEST_IMAGE"
	ENV_TEST_BLOCK_IMAGE                    = "_E2E_TEST_BLOCK_IMAGE"
	ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME = "_E2E_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME"
	ENV_IBOXREPLICA_REMOTE_POOL_ID          = "_E2E_IBOXREPLICA_REMOTE_POOL_ID"
	ENV_K8S_VERSION                         = "_E2E_K8S_VERSION"
	ENV_OCP_VERSION                         = "_E2E_OCP_VERSION"
	ENV_USE_NFS_V4                          = "_E2E_USE_NFS_V4"
	ENV_NFS_EXPORT_PERMISSION               = "_E2E_NFS_EXPORT_PERMISSION"
	ENV_STRESS_ITERATIONS                   = "_E2E_STRESS_ITERATIONS"
	ENV_STRESS_SLEEP_SECONDS                = "_E2E_STRESS_SLEEP_SECONDS"
)

type TestConfig struct {
	NetworkSpaceToUse     string
	NetworkSpaceToUse2    string
	NFSPermissions        string
	Protocol              string
	NodeName              string
	Testt                 *testing.T
	ClientSet             *kubernetes.Clientset
	DynamicClient         *dynamic.DynamicClient
	SnapshotClient        *snapshotv6.Clientset
	GroupSnapshotClient   *groupsnapshotv1beta1.GroupsnapshotV1beta1Client
	RestConfig            *rest.Config
	UsePVCVolumeRef       bool
	UseFsGroup            bool
	UseBlock              bool
	UseAntiAffinity       bool
	UseRetainStorageClass bool
	UseSnapdirVisible     bool
	ReadOnlyPod           bool
	ReadOnlyPodVolume     bool
	UseNFSV4              bool
	UseSELinux            bool
	UseSnapshot           bool
	UseSnapshotLock       bool
	StressIterations      int
	StressSleepSeconds    int
	PVCAnnotations        *PVCAnnotations
	AccessMode            v1.PersistentVolumeAccessMode
	TestNames             *TestResourceNames
	ClientService         *api.ClientService
}

func GetTestConfig(t *testing.T, protocol string) (config *TestConfig, err error) {
	GetFlags(t)

	config = &TestConfig{}

	config.TestNames = &TestResourceNames{}
	config.TestNames.UniqueSuffix = RandSeq(3)
	e2eNamespace := fmt.Sprintf(E2E_NAMESPACE, protocol)
	config.TestNames.NSName = e2eNamespace + config.TestNames.UniqueSuffix
	scName := fmt.Sprintf(SC_NAME, protocol)
	config.TestNames.SCName = scName + config.TestNames.UniqueSuffix
	config.TestNames.VSCName = scName + config.TestNames.UniqueSuffix
	config.TestNames.PVCName = fmt.Sprintf(PVC_NAME, protocol)

	//connect to kube
	err = GetKubeClient(config, *KubeConfigPath)
	if err != nil {
		return nil, err
	}

	if config.ClientSet == nil {
		return nil, fmt.Errorf("error getting ClientSet")
	}

	config.RestConfig = GetRestConfig(*KubeConfigPath)

	if config.RestConfig == nil {
		return nil, fmt.Errorf("error getting RESTConfig")
	}

	config.AccessMode = v1.ReadWriteOnce
	config.Testt = t
	config.Protocol = protocol

	hostname := os.Getenv(ENV_IBOX_HOSTNAME)
	if hostname == "" {
		return config, fmt.Errorf("%s env var required", ENV_IBOX_HOSTNAME)
	}
	username := os.Getenv(ENV_IBOX_USERNAME)
	if username == "" {
		return config, fmt.Errorf("%s env var required", ENV_IBOX_USERNAME)
	}
	password := os.Getenv(ENV_IBOX_PASSWORD)
	if password == "" {
		return config, fmt.Errorf("%s env var required", ENV_IBOX_PASSWORD)
	}

	c := make(map[string]string)
	secrets := map[string]string{
		"hostname": hostname,
		"password": password,
		"username": username,
	}

	x := api.ClientService{
		ConfigMap:  c,
		SecretsMap: secrets,
	}

	config.ClientService, err = x.NewClient()
	if err != nil {
		return config, err
	}

	return config, nil
}

var zlog = log.Get() // grab the logger for package use

func SetupControllerClient() (pb.ControllerClient, error) {
	host, err := GetKubeHost()
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	grpcAddress := fmt.Sprintf("%s:%s", host, SOCAT_SERVICE_PORT)
	conn, err := SetupGRPC(grpcAddress)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	cl := pb.NewControllerClient(conn)
	return cl, nil
}

func SetupGRPC(grpcAddress string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	return conn, nil

}
func GetKubeHost() (string, error) {
	kcenv := os.Getenv("KUBECONFIG")
	//zlog.Info().Msgf("KUBECONFIG is %s", kcenv)

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kcenv)
	if err != nil {
		return "", err
	}

	//zlog.Info().Msgf("host is %s", config.Host)
	parts := strings.Split(config.Host, ":")
	if len(parts) < 2 {
		return parts[0], nil
	}
	s := strings.Trim(parts[1], "/")
	//zlog.Info().Msgf("host is %s", s)
	return s, nil
}
