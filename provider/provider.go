package provider

import (
	"infinibox-csi-driver/service"

	"github.com/rexray/gocsi"
)

//New initialise the parameter to controller and nodeserver
func New(config map[string]string) gocsi.StoragePluginProvider {
	srvc := service.New(config)
	return &gocsi.StoragePlugin{
		Controller:  srvc,
		Node:        srvc,
		Identity:    srvc,
		BeforeServe: srvc.BeforeServe,
		EnvVars: []string{
			// Enable request validation
			gocsi.EnvVarSpecReqValidation + "=true",

			// Enable serial volume access
			gocsi.EnvVarSerialVolAccess + "=true",
		},
	}
}
