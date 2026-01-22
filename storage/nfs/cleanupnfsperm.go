package nfs

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
)

// removed unused NFS permissions for a given IP (node) if there
// are no mounts on the node any longer
// this function is to be called after the unmount has completed
func cleanupNFSPerms(ctx context.Context, volumeID int) {
	// get the node name and IP which we'l use for identifying this node
	nodeName := os.Getenv(common.EnvVarKubeNodeName)
	nodeIP := os.Getenv(common.EnvVarNodeIP)
	slog.Debug("info", "volumeID", volumeID, "node name", nodeName, "node ip", nodeIP)

	// get a connection to the kube api
	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		slog.Error("could not get kube client", "error", err.Error())
		return
	}

	// find the PV for this volume (filesystem), the volumeHandle in the PV
	// contains the volumeID which allows us to find the correct PV, we
	// use the PV to obtain the ibox credentials used to create the volume
	// this is necessary because the unmount stage of CSI doesn't pass the
	// ibox credentials down as secrets as other CSI stages do
	persistentVolume, err := kubeClient.GetPVByVolumeID(ctx, volumeID, common.ProtocolNFS)
	if err != nil {
		slog.Error("could not get pv by volumeID", "error", err.Error())
		return
	}
	slog.Debug("pv by volumeID", "name", persistentVolume.Name)
	secretMap, err := kubeClient.GetSecret(ctx, persistentVolume.Spec.CSI.ControllerExpandSecretRef.Name, persistentVolume.Spec.CSI.ControllerExpandSecretRef.Namespace)
	if err != nil {
		slog.Error("could not get kube secret", "error", err.Error())
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
		slog.Error("error getting ClientService", "error", err.Error())
		return
	}

	fileSystem, err = clientService.IboxAPI.GetFileSystemByID(ctx, volumeID)
	if err != nil {
		slog.Error("error GetFileSystemByID", "volumeid", volumeID, "error", err.Error())
		return
	}
	slog.Debug("looked up fs name", "fsname", fileSystem.Name)

	exports, err = clientService.IboxAPI.GetExportsByFileSystemID(ctx, volumeID)
	if err != nil {
		slog.Error("error GetExportsByFileSystemID", "volumeid", volumeID, "error", err.Error())
		return
	}

	exportCount := len(exports)
	if exportCount == 0 {
		slog.Debug("no exports found for volumeID, no need to cleanup export rule perms for this ip", "volumeid", volumeID)
		return
	}

	slog.Error("found exports for volumeID", "len", exportCount, "volumeid", volumeID)
	// call nfsstat on this node to get the mounted volumes
	// get nfsstats mount information for this node,look for lines that
	// have the 'kube' string and the file system name in them
	nfsstatCommand := "nfsstat -m"
	slog.Debug("info", "command", nfsstatCommand)
	out, _, err := storagecommon.ExecCommand.Command(nfsstatCommand, "")
	if err != nil {
		slog.Error("error executing nfsstat", "error", err.Error())
		return
	}
	slog.Debug("nfsstat", "output", strings.TrimSpace(out))

	volumeMounted := isVolumeMounted(out, fileSystem.Name)
	slog.Debug("info", "volumeMounted", volumeMounted)

	// only perform this logic if there are no more mounts for this volume
	// on this kube node
	if volumeMounted {
		return
	}

	// find the right export in the list
	for _, export := range exports {
		numPermissions := len(export.Permissions)
		slog.Debug("export", "exportPath", export.ExportPath, "export perms", export.Permissions, "num perms", numPermissions)

		// look for the node ip
		var foundNodeIP bool
		var foundNodeIPIndex int
		for index, permission := range export.Permissions {
			if permission.Client == nodeIP {
				// node ip permission found
				slog.Debug("node ip found in permissions, delete this perm!", "node ip", nodeIP)
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
				_, err := clientService.IboxAPI.DeleteExport(ctx, export.ID)
				if err != nil {
					slog.Error("error deleting export", "export id", export.ID)
					return
				}
				slog.Debug("deleted export succeeded for fs", "export id", export.ID, "fs name", fileSystem.Name)
			}
		} else if len(export.Permissions) > 1 {
			// in this case we seletively delete the ip address permission
			// by updating the export with updated permissions list
			if foundNodeIP {
				updatedPerms := slices.Delete(export.Permissions, foundNodeIPIndex, foundNodeIPIndex+1)
				slog.Debug("info", "originalPerms", export.Permissions, "updatedPerms", updatedPerms)
				exportPathRef := iboxapi.ExportPathRef{
					Permissions: updatedPerms,
				}
				_, err = clientService.IboxAPI.UpdateExportPermissions(ctx, export, exportPathRef)
				if err != nil {
					slog.Error("error updating export permissions", "error", err.Error())
				}
			}
		}
	}
}

func isVolumeMounted(nfsstatOutput string, fsName string) bool {
	lines, err := storagecommon.StringToLines(nfsstatOutput)
	if err != nil {
		slog.Error("error splitting nfsstat output", "output", nfsstatOutput, "error", err.Error())
		return false
	}
	slog.Debug("isVolumeMounted", "nfsstat lines", lines)

	for k, v := range lines {
		if strings.Contains(v, fsName) {
			slog.Debug("isVolumeMounted - found", "fsName", fsName, "index", k, "line", v)
			return true
		}
	}

	return false
}

// determine if the installation has enabled the
// cleanup NFS perms feature
func isCleanupNFSPermsSet() bool {
	envVarText := os.Getenv(common.EnvVarCleanupNFSPerms)
	if envVarText == "" {
		return false
	}
	boolValue, err := strconv.ParseBool(envVarText)
	if err != nil {
		slog.Error("error converting env var to boolean, defaulting to false", "env var", envVarText, "error", err.Error())
		return false
	}
	return boolValue
}
