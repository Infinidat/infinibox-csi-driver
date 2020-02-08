package main

import (
	"context"
	"infinibox-csi-driver/provider"
	"infinibox-csi-driver/service"
	"infinibox-csi-driver/storage"

	"github.com/rexray/gocsi"
	csictx "github.com/rexray/gocsi/context"
	log "github.com/sirupsen/logrus"
)

//starting method of CSI-Driver
func main() {
	configParams := make(map[string]string)
	if nodeid, ok := csictx.LookupEnv(context.Background(), "KUBE_NODE_NAME"); ok {
		storage.NodeId = nodeid
	}
	if drivername, ok := csictx.LookupEnv(context.Background(), "CSI_DRIVER_NAME"); ok {
		configParams["drivername"] = drivername
	}

	if logLevel, ok := csictx.LookupEnv(context.Background(), "APP_LOG_LEVEL"); ok {
		configureLog(logLevel)
	}
	if nodeip, ok := csictx.LookupEnv(context.Background(), "NODE_IP_ADDRESS"); ok {
		configParams["nodeIPAddress"] = nodeip
		configParams["nodeid"] = nodeip
		storage.NodeId = nodeip
	}

	gocsi.Run(
		context.Background(),
		service.ServiceName,
		"A Infinibox CSI Driver Plugin",
		usage,
		provider.New(configParams))

}

// set global log level
func configureLog(logLevel string) {
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Error("Invalid logging level: ", logLevel)
		ll = log.InfoLevel // to be set to error level
	}
	log.SetLevel(ll)
	log.Debug("Logging  level set to ", log.GetLevel().String())
}

const usage = `   `
