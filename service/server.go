/*
Copyright 2026 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"fmt"
	"log/slog"
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
		slog.Error(err.Error())
		os.Exit(1)
	}

	if proto == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			slog.Error("Failed to remove addr", "addr", addr, "error", err.Error())
			os.Exit(1)
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
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

	slog.Debug("Listening on address", "address", listener.Addr().String())

	err = server.Serve(listener)
	if err != nil {
		slog.Error("Failed to serve grpc server", "error", err)
		os.Exit(1)
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
