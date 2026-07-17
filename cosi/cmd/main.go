package main

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

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/cosi/internal/s3"
	"github.com/infinidat/infinibox-csi-driver/cosi/pkg/driver"
	cosi "sigs.k8s.io/container-object-storage-interface/proto"
)

type runOptions struct {
	driverName   string
	cosiEndpoint string
	kubeconfig   string
	creds        map[string]string
}

var version string
var compileDate string
var gitHash string

func main() {
	flag.Parse()

	ThisLogger := common.SetupSlog(false)

	// Set the default logger
	slog.SetDefault(ThisLogger)

	// this call effectively initializes the logging system based on environment variables
	// and default configurations.

	slog.Info("Infinidat COSI Driver is Starting", "version", version, "compile date", compileDate, "git hash", gitHash, "log level", os.Getenv("APP_LOG_LEVEL"), "compiler version", runtime.Version())

	opts := runOptions{
		cosiEndpoint: defaultEnv("COSI_ENDPOINT", "unix:///var/lib/cosi/cosi.sock"),
		driverName:   defaultEnv("X_COSI_DRIVER_NAME", "cosi-driver.infinidat.com"),
		kubeconfig:   defaultEnv("KUBECONFIG", ""),
	}

	var err error
	opts.creds, err = s3.GetS3Credentials()
	if err != nil {
		slog.Error("Exiting CREDENTIALS_SECRET_NAME is not set or problem getting creds", "error", err)
		os.Exit(1)

	}

	if err := run(context.Background(), opts); err != nil {
		slog.Error("Exiting on error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts runOptions) error {
	ctx, stop := signal.NotifyContext(ctx,
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	kcli, err := NewClient(opts.kubeconfig)
	if err != nil {
		return fmt.Errorf("unable to create Kubernetes client: %w", err)
	}

	identityServer := &driver.IdentityServer{DriverName: opts.driverName}
	provisionerServer := &driver.ProvisionerServer{
		DynamicClient: s3.NewFactory(kcli).NewClient,
		S3Credentials: opts.creds,
	}

	server, err := grpcServer(identityServer, provisionerServer)
	if err != nil {
		return fmt.Errorf("gRPC server creation failed: %w", err)
	}

	lis, cleanup, err := listener(ctx, opts.cosiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to create listener for %s: %w", opts.cosiEndpoint, err)
	}
	defer cleanup()

	var wg sync.WaitGroup
	wg.Add(1)
	go shutdown(ctx, &wg, server)

	if err = server.Serve(lis); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}

	wg.Wait()
	return nil
}

func listener(
	ctx context.Context,
	cosiEndpoint string,
) (net.Listener, func(), error) {
	endpointURL, err := url.Parse(cosiEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse COSI endpoint: %w", err)
	}

	listenConfig := net.ListenConfig{}

	if endpointURL.Scheme == "unix" {
		_ = os.Remove(endpointURL.Path) // Cleanup stale socket
	}

	listener, err := listenConfig.Listen(ctx, endpointURL.Scheme, endpointURL.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create listener: %w", err)
	}

	cleanup := func() {
		if endpointURL.Scheme == "unix" {
			if err := os.Remove(endpointURL.Path); err != nil {
				slog.Error("Failed to remove old socket", "error", err)
			}
		}
	}

	return listener, cleanup, nil
}

func grpcServer(
	identity cosi.IdentityServer,
	provisioner cosi.ProvisionerServer,
) (*grpc.Server, error) {
	if identity == nil || provisioner == nil {
		return nil, errors.New("identity and provisioner servers cannot be nil")
	}

	server := grpc.NewServer()

	cosi.RegisterIdentityServer(server, identity)
	cosi.RegisterProvisionerServer(server, provisioner)

	return server, nil
}

const (
	gracePeriod = 5 * time.Second
)

func shutdown(
	ctx context.Context,
	wg *sync.WaitGroup,
	g *grpc.Server,
) {
	<-ctx.Done()
	defer wg.Done()
	defer slog.Info("Stopped")

	slog.Info("Shutting down")

	dctx, stop := context.WithTimeout(context.Background(), gracePeriod)
	defer stop()

	c := make(chan struct{}, 1)

	if g != nil {
		go func() {
			g.GracefulStop()
			c <- struct{}{}
		}()

		for {
			select {
			case <-dctx.Done():
				slog.Info("Forcing shutdown")
				g.Stop()
				return
			case <-c:
				return
			}
		}
	}
}

/**
func asBool(v string) bool {
	b, _ := strconv.ParseBool(v)
	return b
}
*/

func NewClient(kubeconfig string) (kubernetes.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig from %q: %w", kubeconfig, err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return clientset, nil
}

func defaultEnv(key, defaultValue string) string {
	val, found := os.LookupEnv(key)
	if !found || val == "" {
		return defaultValue
	}

	return strings.TrimSpace(val)
}
