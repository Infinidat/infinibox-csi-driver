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
	"infinibox-csi-driver/provider"
	"infinibox-csi-driver/service"

	"github.com/rexray/gocsi"
	csictx "github.com/rexray/gocsi/context"
)

//starting method of CSI-Driver
func main() {
	configParams := getConfigParams()
	gocsi.Run(
		context.Background(),
		service.ServiceName,
		"A Infinibox CSI Driver Plugin",
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
