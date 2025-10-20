package nfs

import (
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/api/clientgo"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	storagecommon "infinibox-csi-driver/storage/common"
	"os"
	"slices"
	"strconv"
	"strings"
)

// removed unused NFS permissions for a given IP (node) if there
// are no mounts on the node any longer
// this function is to be called after the unmount has completed
func cleanupNFSPerms(volumeID int) {
	const functionName = "cleanupNFSPerms"

	// get the node name and IP which we'l use for identifying this node
	nodeName := os.Getenv(common.ENV_VAR_KUBE_NODE_NAME)
	nodeIP := os.Getenv(common.ENV_VAR_NODE_IP)
	zlog.Debug().Msgf("%s - volumeID %d node %s node IP %s", functionName, volumeID, nodeName, nodeIP)

	// get a connection to the kube api
	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		zlog.Error().Msgf("%s - could not get kube client %s", functionName, err.Error())
		return
	}

	// find the PV for this volume (filesystem), the volumeHandle in the PV
	// contains the volumeID which allows us to find the correct PV, we
	// use the PV to obtain the ibox credentials used to create the volume
	// this is necessary because the unmount stage of CSI doesn't pass the
	// ibox credentials down as secrets as other CSI stages do
	persistentVolume, err := kubeClient.GetPVByVolumeID(volumeID, common.PROTOCOL_NFS)
	if err != nil {
		zlog.Error().Msgf("%s - could not get pv by volumeID %s", functionName, err.Error())
		return
	} else {
		zlog.Debug().Msgf("%s - pv by volumeID %s", functionName, persistentVolume.Name)
	}
	secretMap, err := kubeClient.GetSecret(persistentVolume.Spec.CSI.ControllerExpandSecretRef.Name, persistentVolume.Spec.CSI.ControllerExpandSecretRef.Namespace)
	if err != nil {
		zlog.Error().Msgf("%s - could not get kube secret %s", functionName, err.Error())
		return
	}

	var exports []iboxapi.Export
	var fileSystem *iboxapi.FileSystem

	// get an ibox api connection using this secret
	client := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secretMap,
	}

	clientService, err := client.NewClient()
	if err != nil {
		zlog.Error().Msgf("%s - error getting ClientService %s", functionName, err.Error())
		return
	}

	fileSystem, err = clientService.IboxAPI.GetFileSystemByID(volumeID)
	if err != nil {
		zlog.Error().Msgf("%s - error GetFileSystemByID volumeID %d error %s", functionName, volumeID, err.Error())
		return
	}
	zlog.Debug().Msgf("%s - looked up fs name %s", functionName, fileSystem.Name)

	exports, err = clientService.IboxAPI.GetExportsByFileSystemID(volumeID)
	if err != nil {
		zlog.Error().Msgf("%s error GetExportsByFileSystemID volumeID %d error %s", functionName, volumeID, err.Error())
		return
	}

	if len(exports) == 0 {
		zlog.Debug().Msgf("%s - no exports found for volumeID %d, no need to cleanup export rule perms for this ip", functionName, volumeID)
		return
	}

	if len(exports) > 0 {
		zlog.Error().Msgf("%s - found %d exports for volumeID %d", functionName, len(exports), volumeID)
		// call nfsstat on this node to get the mounted volumes
		// get nfsstats mount information for this node,look for lines that
		// have the 'kube' string and the file system name in them
		nfsstatCommand := "nfsstat -m"
		zlog.Debug().Msgf("%s", nfsstatCommand)
		out, _, err := storagecommon.ExecCommand.Command(nfsstatCommand, "")
		if err != nil {
			zlog.Error().Msgf("%s  - error executing nfsstat %s", functionName, err.Error())
			return
		}
		zlog.Debug().Msgf("%s - nfsstat output is [%s]", functionName, strings.TrimSpace(string(out)))

		volumeMounted := isVolumeMounted(out, fileSystem.Name)
		zlog.Debug().Msgf("%s - volumeMounted [%t]", functionName, volumeMounted)

		// only perform this logic if there are no more mounts for this volume
		// on this kube node
		if !volumeMounted {
			// find the right export in the list
			for _, export := range exports {
				numPermissions := len(export.Permissions)
				zlog.Debug().Msgf("%s - export - exportPath %s permissions [%v] numPermissions %d", functionName, export.ExportPath, export.Permissions, numPermissions)

				// look for the node ip
				var foundNodeIP bool
				var foundNodeIPIndex int
				for index, permission := range export.Permissions {
					if permission.Client == nodeIP {
						// node ip permission found
						zlog.Debug().Msgf("%s - node ip %s found in permissions, delete this perm!", functionName, nodeIP)
						foundNodeIP = true
						foundNodeIPIndex = index
					}
				}
				if numPermissions == 1 {
					// in this case, we can just delete the export entirely since
					// you can't have an export with zero permissions
					// if the filesystem is remounted ever it will cause a new
					// export to be created
					if foundNodeIP {
						_, err := clientService.IboxAPI.DeleteExport(export.ID)
						if err != nil {
							zlog.Error().Msgf("%s - error deleting export %d", functionName, export.ID)
							return
						}
						zlog.Debug().Msgf("%s - deleted export %d succeeded for fs %s", functionName, export.ID, fileSystem.Name)
					}
				} else if len(export.Permissions) > 1 {
					// in this case we seletively delete the ip address permission
					// by updating the export with updated permissions list
					if foundNodeIP {
						zlog.Debug().Msgf("%s - originalPerms [%v]", functionName, export.Permissions)
						updatedPerms := slices.Delete(export.Permissions, foundNodeIPIndex, foundNodeIPIndex+1)
						zlog.Debug().Msgf("%s - updatedPerms [%v]", functionName, updatedPerms)
						exportPathRef := iboxapi.ExportPathRef{
							Permissions: updatedPerms,
						}
						_, err = clientService.IboxAPI.UpdateExportPermissions(export, exportPathRef)
						if err != nil {
							zlog.Error().Msgf("%s - error updating export permissions %s", functionName, err.Error())
						}
					}
				}
			}
		}
	}
}

func isVolumeMounted(nfsstatOutput string, fsName string) bool {
	lines, err := storagecommon.StringToLines(nfsstatOutput)
	if err != nil {
		zlog.Error().Msgf("isVolumeMounted - error splitting nfsstat output %s error %s", nfsstatOutput, err.Error())
		return false
	}
	zlog.Debug().Msgf("isVolumeMounted - nfsstat lines = %v", lines)

	for k, v := range lines {
		if strings.Contains(v, fsName) {
			zlog.Debug().Msgf("isVolumeMounted - found fsName %s on line %d = %s", fsName, k, v)
			return true
		}
	}

	return false
}

// determine if the installation has enabled the
// cleanup NFS perms feature
func isCleanupNFSPermsSet() bool {
	envVarText := os.Getenv(common.ENV_VAR_CLEANUP_NFS_PERMS)
	if envVarText == "" {
		return false
	}
	boolValue, err := strconv.ParseBool(envVarText)
	if err != nil {
		zlog.Error().Msgf("isCleanupNFSPerms - error converting env var %s to boolean, defaulting to false - error %s", envVarText, err.Error())
		return false
	}
	return boolValue
}
