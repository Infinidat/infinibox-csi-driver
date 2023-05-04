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
	"flag"
	"infinibox-csi-driver/service"
	"os"

	"k8s.io/klog/v2"
)

var version string
var compileDate string
var gitHash string

// starting method of CSI-Driver
func main() {
	defer klog.Flush() // Flush pending log IO
	klog.InitFlags(nil)
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("stderrthreshold", "WARNING")

	var verbosity string
	appLogLevel := os.Getenv("APP_LOG_LEVEL")
	switch appLogLevel {
	case "quiet":
		verbosity = "1"
	case "info":
		verbosity = "2"
	case "extended":
		verbosity = "3"
	case "debug":
		verbosity = "4"
	case "trace":
		verbosity = "5"
	default:
		verbosity = "2"
	}
	_ = flag.Set("v", verbosity)
	flag.Parse()

	klog.V(2).Infof("Infinidat CSI Driver is Starting")
	klog.V(2).Infof("Version: %s", version)
	klog.V(2).Infof("Compile date: %s", compileDate)
	klog.V(2).Infof("Compile git hash: %s", gitHash)
	klog.V(2).Infof("Log level: %s", appLogLevel)
	klog.Flush()

	nodeIP := os.Getenv("NODE_IP")
	if nodeIP == "" {
		klog.Error("NODE_IP not set")
		os.Exit(1)
	}
	driverName := os.Getenv("CSI_DRIVER_NAME")
	if driverName == "" {
		klog.Error("CSI_DRIVER_NAME not set")
		os.Exit(1)
	}
	csiEndpoint := os.Getenv("CSI_ENDPOINT")
	if csiEndpoint == "" {
		klog.Error("CSI_ENDPOINT not set")
		os.Exit(1)
	}
	if version == "" {
		klog.Error("version not set")
		os.Exit(1)
	}

	klog.V(2).Infof("NodeIP: %s", nodeIP)
	klog.V(2).Infof("DriverName: %s", driverName)
	klog.V(2).Infof("Endpoint: %s", csiEndpoint)
	klog.V(2).Infof("Version: %s", version)

	driverOptions := service.DriverOptions{
		NodeID:     nodeIP,
		DriverName: driverName,
		Endpoint:   csiEndpoint,
		Version:    version,
	}
	d := service.NewDriver(&driverOptions)
	d.Run(false)
}
