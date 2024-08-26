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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	SOCAT_SERVICE_PORT = "30007"
)

type TestConfig struct {
	Protocol              string
	Testt                 *testing.T
	ClientSet             *kubernetes.Clientset
	DynamicClient         *dynamic.DynamicClient
	SnapshotClient        *snapshotv6.Clientset
	RestConfig            *rest.Config
	UsePVCVolumeRef       bool
	UseFsGroup            bool
	UseBlock              bool
	UseAntiAffinity       bool
	UseRetainStorageClass bool
	UseSnapdirVisible     bool
	ReadOnlyPod           bool
	ReadOnlyPodVolume     bool
	UseSELinux            bool
	UseSnapshot           bool
	UseSnapshotLock       bool
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
	config.ClientSet, config.DynamicClient, config.SnapshotClient, err = GetKubeClient(*KubeConfigPath)

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

	hostname := os.Getenv("_E2E_IBOX_HOSTNAME")
	if hostname == "" {
		return config, fmt.Errorf("_E2E_IBOX_HOSTNAME env var required")
	}
	username := os.Getenv("_E2E_IBOX_USERNAME")
	if username == "" {
		return config, fmt.Errorf("_E2E_IBOX_USERNAME env var required")
	}
	password := os.Getenv("_E2E_IBOX_PASSWORD")
	if password == "" {
		return config, fmt.Errorf("_E2E_IBOX_PASSWORD env var required")
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
	conn, err := grpc.Dial(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
