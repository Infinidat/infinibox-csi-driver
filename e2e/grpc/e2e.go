package grpc

import (
	"fmt"
	"os"
	"strings"

	pb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	SOCAT_SERVICE_PORT = "30007"
)

func SetupControllerClient() (pb.ControllerClient, error) {
	host, err := GetKubeHost()
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	grpcAddress := fmt.Sprintf("%s:%s", host, SOCAT_SERVICE_PORT)
	conn, err := SetupGRPC(grpcAddress)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	cl := pb.NewControllerClient(conn)
	return cl, nil
}

func SetupGRPC(grpcAddress string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	return conn, nil

}
func GetKubeHost() (string, error) {
	kcenv := os.Getenv("KUBECONFIG")
	klog.V(4).Infof("KUBECONFIG is %s\n", kcenv)

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kcenv)
	if err != nil {
		return "", err
	}

	klog.V(4).Infof("host is %s\n", config.Host)
	parts := strings.Split(config.Host, ":")
	if len(parts) < 2 {
		return parts[0], nil
	}
	s := strings.Trim(parts[1], "/")
	klog.V(4).Infof("host is %s\n", s)
	return s, nil
}
