package e2e

import (
	"fmt"
	"os"
	"strings"

	"infinibox-csi-driver/log"

	pb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	SOCAT_SERVICE_PORT = "30007"
)

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
	conn, err := grpc.Dial(grpcAddress, grpc.WithInsecure())
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
