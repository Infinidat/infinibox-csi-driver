/*
Copyright 2022 Infinidat
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
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/log"
	"github.com/infinidat/infinibox-csi-driver/service"

	v1 "k8s.io/api/core/v1"
)

var version string
var compileDate string
var gitHash string

// starting method of CSI-Driver
func main() {
	appLogLevel := os.Getenv("APP_LOG_LEVEL")
	var logLevel slog.Leveler
	switch appLogLevel {
	case "error":
		logLevel = slog.LevelError
	case "warn":
		logLevel = slog.LevelWarn
	case "info":
		logLevel = slog.LevelInfo
	case "debug":
		logLevel = slog.LevelDebug
	case "trace":
		logLevel = common.LevelTrace
	default:
		logLevel = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{
		Level:       logLevel,
		AddSource:   true,
		ReplaceAttr: common.CustomSlogFormatter,
	}
	ThisLogger := slog.New(slog.NewJSONHandler(os.Stdout, opts))

	// Set the default logger
	slog.SetDefault(ThisLogger)

	// this call effectively initializes the logging system based on environment variables
	// and default configurations.

	slog.Info("Infinidat CSI Driver is Starting", "version", version, "compile date", compileDate, "git hash", gitHash, "log level", os.Getenv("APP_LOG_LEVEL"))

	log.SetupKlog()

	nodeIP := os.Getenv("NODE_IP")
	if nodeIP == "" {
		slog.Error("NODE_IP not set")
		os.Exit(1)
	}
	driverName := os.Getenv("CSI_DRIVER_NAME")
	if driverName == "" {
		slog.Error("CSI_DRIVER_NAME not set")
		os.Exit(1)
	}
	csiEndpoint := os.Getenv("CSI_ENDPOINT")
	if csiEndpoint == "" {
		slog.Error("CSI_ENDPOINT not set")
		os.Exit(1)
	}
	if version == "" {
		slog.Error("version not set")
		os.Exit(1)
	}

	ctx := context.Background()

	node, nodeCount, err := getKubeNode(ctx)
	if err != nil {
		slog.Error("error in getting kube node", "error", err.Error())
		os.Exit(1)
	}

	osVersion := node.Status.NodeInfo.OSImage
	kubeVersion := node.Status.NodeInfo.KubeletVersion

	// set the env vars so we can get it in other parts of the driver to create events
	err = os.Setenv(common.EnvVarCSIDriverVersion, version)
	if err != nil {
		slog.Error("error in setenv", "error", err.Error())
		os.Exit(1)
	}
	err = os.Setenv(common.EnvVarOSVersion, osVersion)
	if err != nil {
		slog.Error("error in setenv", "error", err.Error())
		os.Exit(1)
	}
	err = os.Setenv(common.EnvVarKubeVersion, kubeVersion)
	if err != nil {
		slog.Error("error in setenv", "error", err.Error())
		os.Exit(1)
	}
	err = os.Setenv(common.EnvVarNodeCount, nodeCount)
	if err != nil {
		slog.Error("error in setenv", "error", err.Error())
		os.Exit(1)
	}

	slog.Info("versionInfo", "NodeIP", nodeIP, "KubeNodeName", os.Getenv("KUBE_NODE_NAME"), "DriverName", driverName, "Endpoint", csiEndpoint)
	slog.Info("versionInfo continued", "Version", version, "OS Version", osVersion, "Kube Version", kubeVersion, "Kube Node Count", nodeCount)

	driverOptions := service.DriverOptions{
		NodeID:     nodeIP,
		DriverName: driverName,
		Endpoint:   csiEndpoint,
		Version:    version,
	}
	d := service.NewDriver(&driverOptions)
	d.Run(false)
}

func getKubeNode(ctx context.Context) (node v1.Node, nodeCount string, err error) {
	kc, err := clientgo.BuildClient()
	if err != nil {
		return node, "", err
	}
	nodes, err := kc.GetNodes(ctx)
	if len(nodes) == 0 {
		return node, "", fmt.Errorf("zero nodes found, problem getting a node")
	}

	// assumption is that all kube nodes are running the same version
	return nodes[0], strconv.Itoa(len(nodes)), err
}
