package storage

import (
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/api/clientgo"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	"os"
	"slices"
	"strconv"
	"strings"
)

// removed unused NFS permissions for a given IP (node) if there
// are no mounts on the node any longer
// this function is to be called after the unmount has completed
func cleanupNFSPerms(volumeID int) {

	const FN = "cleanupNFSPerms"

	// get the node name and IP which we'l use for identifying this node
	nodeName := os.Getenv("KUBE_NODE_NAME")
	nodeIP := os.Getenv("NODE_IP")
	zlog.Debug().Msgf("%s - volumeID %d node %s node IP %s", FN, volumeID, nodeName, nodeIP)

	// get a connection to the kube api
	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		zlog.Error().Msgf("%s - could not get kube client %s", FN, err.Error())
		return
	}

	// find the PV for this volume (filesystem), the volumeHandle in the PV
	// contains the volumeID which allows us to find the correct PV, we
	// use the PV to obtain the ibox credentials used to create the volume
	// this is necessary because the unmount stage of CSI doesn't pass the
	// ibox credentials down as secrets as other CSI stages do
	pv, err := kubeClient.GetPVByVolumeID(volumeID, common.PROTOCOL_NFS)
	if err != nil {
		zlog.Error().Msgf("%s - could not get pv by volumeID %s", FN, err.Error())
		return
	} else {
		zlog.Debug().Msgf("%s - pv by volumeID %s", FN, pv.Name)
	}
	secretMap, err := kubeClient.GetSecret(pv.Spec.CSI.ControllerExpandSecretRef.Name, pv.Spec.CSI.ControllerExpandSecretRef.Namespace)
	if err != nil {
		zlog.Error().Msgf("%s - could not get kube secret %s", FN, err.Error())
		return
	}

	var exports []iboxapi.Export
	var fs *iboxapi.FileSystem

	// get an ibox api connection using this secret
	x := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secretMap,
	}

	clientsvc, err := x.NewClient()
	if err != nil {
		zlog.Error().Msgf("%s - error getting ClientService %s", FN, err.Error())
		return
	}

	fs, err = clientsvc.Iboxapi.GetFileSystemByID(volumeID)
	if err != nil {
		zlog.Error().Msgf("%s - error GetFileSystemByID volumeID %d error %s", FN, volumeID, err.Error())
		return
	}
	zlog.Debug().Msgf("%s - looked up fs name %s", FN, fs.Name)

	exports, err = clientsvc.Iboxapi.GetExportsByFileSystemID(volumeID)
	if err != nil {
		zlog.Error().Msgf("%s error GetExportsByFileSystemID volumeID %d error %s", FN, volumeID, err.Error())
		return
	}

	if len(exports) == 0 {
		zlog.Debug().Msgf("%s - no exports found for volumeID %d, no need to cleanup export rule perms for this ip", FN, volumeID)
		return
	}

	if len(exports) > 0 {
		zlog.Error().Msgf("%s - found %d exports for volumeID %d", FN, len(exports), volumeID)
		// call nfsstat on this node to get the mounted volumes
		// get nfsstats mount information for this node,look for lines that
		// have the 'kube' string and the file system name in them
		nfsstatCommand := "nfsstat -m"
		zlog.Debug().Msgf("%s", nfsstatCommand)
		out, _, err := execCommand.Command(nfsstatCommand, "")
		if err != nil {
			zlog.Error().Msgf("%s  - error executing nfsstat %s", FN, err.Error())
			return
		}
		zlog.Debug().Msgf("%s - nfsstat output is [%s]", FN, strings.TrimSpace(string(out)))

		volumeMounted := isVolumeMounted(out, fs.Name)
		zlog.Debug().Msgf("%s - volumeMounted [%t]", FN, volumeMounted)

		// only perform this logic if there are no more mounts for this volume
		// on this kube node
		if !volumeMounted {
			// find the right export in the list
			for _, ex := range exports {

				numPermissions := len(ex.Permissions)
				zlog.Debug().Msgf("%s - export - exportPath %s permissions [%v] numPermissions %d", FN, ex.ExportPath, ex.Permissions, numPermissions)

				// look for the node ip
				var foundNodeIP bool
				var foundNodeIPIndex int
				for k, perm := range ex.Permissions {
					if perm.Client == nodeIP {
						// node ip permission found
						zlog.Debug().Msgf("%s - node ip %s found in permissions, delete this perm!", FN, nodeIP)
						foundNodeIP = true
						foundNodeIPIndex = k
					}
				}
				if numPermissions == 1 {
					// in this case, we can just delete the export entirely since
					// you can't have an export with zero permissions
					// if the filesystem is remounted ever it will cause a new
					// export to be created
					if foundNodeIP {
						_, err := clientsvc.Iboxapi.DeleteExport(ex.ID)
						if err != nil {
							zlog.Error().Msgf("%s - error deleting export %d", FN, ex.ID)
							return
						}
						zlog.Debug().Msgf("%s - deleted export %d succeeded for fs %s", FN, ex.ID, fs.Name)
					}
				} else if len(ex.Permissions) > 1 {
					// in this case we seletively delete the ip address permission
					// by updating the export with updated permissions list
					if foundNodeIP {
						zlog.Debug().Msgf("%s - originalPerms [%v]", FN, ex.Permissions)
						updatedPerms := slices.Delete(ex.Permissions, foundNodeIPIndex, len(ex.Permissions))
						zlog.Debug().Msgf("%s - updatedPerms [%v]", FN, updatedPerms)
						exportPathRef := iboxapi.ExportPathRef{
							Permissions: updatedPerms,
						}
						_, err = clientsvc.Iboxapi.UpdateExportPermissions(ex, exportPathRef)
						if err != nil {
							zlog.Error().Msgf("%s - error updating export permissions %s", FN, err.Error())
						}
					}
				}
			}
		}

	}

}

func isVolumeMounted(nfsstatOutput string, fsName string) bool {
	lines, err := stringToLines(nfsstatOutput)
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
	envVarText := os.Getenv("CLEANUP_NFS_PERMS")
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
