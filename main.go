package main

import (
	"context"
	"infinibox-csi-driver/provider"
	"infinibox-csi-driver/service"

	"github.com/rexray/gocsi"
	csictx "github.com/rexray/gocsi/context"
	log "github.com/sirupsen/logrus"
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
	if hostClusterName, ok := csictx.LookupEnv(context.Background(), "HOST_CLUSTER_NAME"); ok {
		configParams["hostclustername"] = hostClusterName
	}
	if logLevel, ok := csictx.LookupEnv(context.Background(), "APP_LOG_LEVEL"); ok {
		configureLog(logLevel)
	}

	if initiatorPrefix, ok := csictx.LookupEnv(context.Background(), "ISCSI_INITIATOR_PREFIX"); ok {
		configParams["initiatorPrefix"] = initiatorPrefix
	}
	return configParams
}

// set global log level
func configureLog(logLevel string) {
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Error("Invalid logging level: ", logLevel)
		ll = log.InfoLevel // to be set to error level
	}
	log.SetLevel(ll)
	log.Info("Logging  level set to ", log.GetLevel().String())

}

const usage = `   `
