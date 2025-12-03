package grpc

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	pb "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	socatServicePort = "30007"
)

func SetupControllerClient() (pb.ControllerClient, error) {
	host, err := GetKubeHost()
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	grpcAddress := fmt.Sprintf("%s:%s", host, socatServicePort)
	conn, err := SetupGRPC(grpcAddress)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	cl := pb.NewControllerClient(conn)
	return cl, nil
}

func SetupGRPC(grpcAddress string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(grpcAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	return conn, nil
}

func GetKubeHost() (string, error) {
	kcenv := os.Getenv("KUBECONFIG")
	slog.Info("KUBECONFIG", "value", kcenv)

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kcenv)
	if err != nil {
		return "", err
	}

	slog.Info("host", "value", config.Host)
	parts := strings.Split(config.Host, ":")
	if len(parts) < 2 {
		return parts[0], nil
	}
	s := strings.Trim(parts[1], "/")
	slog.Info("host", "value", s)
	return s, nil
}
