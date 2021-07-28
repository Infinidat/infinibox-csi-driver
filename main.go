/*Copyright 2020 Infinidat
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
	"github.com/rexray/gocsi"
	csictx "github.com/rexray/gocsi/context"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/provider"
	"infinibox-csi-driver/service"
	"k8s.io/klog"
	"os"
)

//starting method of CSI-Driver
func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Set("stderrthreshold", "WARNING")

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

	flag.Set("v", verbosity)
	flag.Parse()

	klog.V(2).Infof("Infinidat CSI Driver is Starting - v2.1.0-rc1")
	klog.V(2).Infof("Log level: %s", appLogLevel)

	// Check ALLOW_XFS_UUID_REGENERATION
	allow_xfs_uuid_regeneration := os.Getenv("ALLOW_XFS_UUID_REGENERATION")
	_, err := helper.YamlBoolToBool(allow_xfs_uuid_regeneration)
	if err != nil {
		klog.Fatalf("Invalid ALLOW_XFS_UUID_REGENERATION variable: %s", err)
	}
	klog.V(2).Infof("Configuration:")
	klog.V(2).Infof("  ALLOW_XFS_UUID_REGENERATION: %s", allow_xfs_uuid_regeneration)

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
		configParams["nodeip"] = nodeip
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
