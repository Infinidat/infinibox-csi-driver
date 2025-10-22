package service

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/infinidat/infinibox-csi-driver/helper"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
)

// Defines Non blocking GRPC server interfaces
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint string, ids csi.IdentityServer, vgs csi.GroupControllerServer, cs csi.ControllerServer, ns csi.NodeServer, testMode bool)
	// Waits for the service to stop
	Wait()
	// Stops the service gracefully
	Stop()
	// Stops the service forcefully
	ForceStop()
}

func NewNonBlockingGRPCServer() NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	waitGroup sync.WaitGroup
	server    *grpc.Server
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, vgs csi.GroupControllerServer, cs csi.ControllerServer, ns csi.NodeServer, testMode bool) {
	s.waitGroup.Add(1)

	go s.serve(endpoint, ids, vgs, cs, ns, testMode)
}

func (s *nonBlockingGRPCServer) Wait() {
	s.waitGroup.Wait()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(endpoint string, identityServer csi.IdentityServer, groupControllerServer csi.GroupControllerServer, controllerServer csi.ControllerServer, namespace csi.NodeServer, testMode bool) {
	proto, addr, err := ParseEndpoint(endpoint)
	if err != nil {
		zlog.Fatal().Msg(err.Error())
	}

	if proto == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			zlog.Fatal().Msgf("Failed to remove %s, error: %s", addr, err.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		zlog.Fatal().Msgf("Failed to listen: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(helper.LogGRPC),
	}
	server := grpc.NewServer(opts...)
	s.server = server

	if identityServer != nil {
		csi.RegisterIdentityServer(server, identityServer)
	}
	if groupControllerServer != nil {
		csi.RegisterGroupControllerServer(server, groupControllerServer)
	}
	if controllerServer != nil {
		csi.RegisterControllerServer(server, controllerServer)
	}
	if namespace != nil {
		csi.RegisterNodeServer(server, namespace)
	}

	// Used to stop the server while running tests
	if testMode {
		s.waitGroup.Done()
		go func() {
			// make sure Serve() is called
			s.waitGroup.Wait()
			time.Sleep(time.Millisecond * 1000)
			s.server.GracefulStop()
		}()
	}

	zlog.Debug().Msgf("Listening on address: %s", listener.Addr().String())

	err = server.Serve(listener)
	if err != nil {
		zlog.Fatal().Msgf("Failed to serve grpc server: %v", err)
	}
}

func ParseEndpoint(endpoint string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(endpoint), "unix://") || strings.HasPrefix(strings.ToLower(endpoint), "tcp://") {
		endpointParts := strings.SplitN(endpoint, "://", 2)
		if endpointParts[1] != "" {
			return endpointParts[0], endpointParts[1], nil
		}
	}
	return "", "", fmt.Errorf("invalid endpoint: %v", endpoint)
}
