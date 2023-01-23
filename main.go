/*Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package main

import (
	"context"
	"flag"
	"infinibox-csi-driver/provider"
	"infinibox-csi-driver/service"
	"os"

	"github.com/rexray/gocsi"
	csictx "github.com/rexray/gocsi/context"
	"k8s.io/klog"
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

	configParams := getConfigParams()
	gocsi.Run(
		context.Background(),
		service.ServiceName,
		"An Infinibox CSI Driver Plugin",
		usage,
		provider.New(configParams))
}

func getConfigParams() map[string]string {
	configParams := make(map[string]string)
	if nodeip, ok := csictx.LookupEnv(context.Background(), "NODE_IP_ADDRESS"); ok {
		configParams["nodeid"] = nodeip
	}
	if nodeName, ok := csictx.LookupEnv(context.Background(), "KUBE_NODE_NAME"); ok {
		configParams["nodename"] = nodeName
	}
	if drivername, ok := csictx.LookupEnv(context.Background(), "CSI_DRIVER_NAME"); ok {
		configParams["drivername"] = drivername
	}
	if driverversion, ok := csictx.LookupEnv(context.Background(), "CSI_DRIVER_VERSION"); ok {
		configParams["driverversion"] = driverversion
	}
	return configParams
}

const usage = `   `
