/*
Copyright 2023 Infinidat
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
package service

import (
	"log/slog"
	"runtime"

	"github.com/infinidat/infinibox-csi-driver/helper"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/mount-utils"
)

type DriverOptions struct {
	NodeID           string
	DriverName       string
	Endpoint         string
	Version          string
	MountPermissions uint64
	WorkingMountDir  string
}

type Driver struct {
	name             string
	nodeID           string
	version          string
	endpoint         string
	mountPermissions uint64
	workingMountDir  string

	// ids *identityServer
	ns          *NodeServer
	cscap       []*csi.ControllerServiceCapability
	nscap       []*csi.NodeServiceCapability
	groupcap    []*csi.GroupControllerServiceCapability
	volumeLocks *helper.VolumeLocks
}

func NewDriver(options *DriverOptions) *Driver {
	slog.Info("info", "Driver", options.DriverName, "version", options.Version)

	driver := &Driver{
		name:             options.DriverName,
		version:          options.Version,
		nodeID:           options.NodeID,
		endpoint:         options.Endpoint,
		mountPermissions: options.MountPermissions,
		workingMountDir:  options.WorkingMountDir,
	}

	driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,

		/**
		currently unimplemented
		csi.ControllerServiceCapability_RPC_GET_CAPACITY
		csi.ControllerServiceCapability_RPC_PUBLISH_READONLY
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES
		csi.ControllerServiceCapability_RPC_VOLUME_CONDITION
		csi.ControllerServiceCapability_RPC_GET_VOLUME
		csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER
		*/

	})

	driver.AddGroupControllerServiceCapabilities([]csi.GroupControllerServiceCapability_RPC_Type{
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
	})

	driver.AddNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_UNKNOWN,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,

		/**
		currently unimplemented
		csi.NodeServiceCapability_RPC_VOLUME_MOUNT_GROUP,
		csi.NodeServiceCapability_RPC_VOLUME_CONDITION
		csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER
		*/
	})
	driver.volumeLocks = helper.NewVolumeLocks()
	return driver
}

func NewNodeServer(driver *Driver, mounter mount.Interface) *NodeServer {
	return &NodeServer{
		Driver:  driver,
		mounter: mounter,
	}
}

func (driver *Driver) Run(testMode bool) {
	mounter := mount.New("")
	if runtime.GOOS == "linux" {
		// MounterForceUnmounter is only implemented on Linux now
		mounter = mounter.(mount.MounterForceUnmounter)
	}
	driver.ns = NewNodeServer(driver, mounter)
	server := NewNonBlockingGRPCServer()
	server.Start(driver.endpoint,
		NewDefaultIdentityServer(driver),
		NewVolumeGroupServer(driver),
		NewControllerServer(driver),
		driver.ns,
		testMode)
	server.Wait()
}

func NewVolumeGroupServer(driver *Driver) *VolumeGroupServer {
	return &VolumeGroupServer{
		Driver: driver,
	}
}

func NewDefaultIdentityServer(driver *Driver) *IdentityServer {
	return &IdentityServer{
		Driver: driver,
	}
}

func NewControllerServer(driver *Driver) *ControllerServer {
	return &ControllerServer{
		Driver: driver,
	}
}

func (driver *Driver) AddControllerServiceCapabilities(capability []csi.ControllerServiceCapability_RPC_Type) {
	csc := []*csi.ControllerServiceCapability{}
	for _, c := range capability {
		csc = append(csc, NewControllerServiceCapability(c))
	}
	driver.cscap = csc
}

func (driver *Driver) AddGroupControllerServiceCapabilities(capability []csi.GroupControllerServiceCapability_RPC_Type) {
	csc := []*csi.GroupControllerServiceCapability{}
	for _, c := range capability {
		csc = append(csc, NewGroupControllerServiceCapability(c))
	}
	driver.groupcap = csc
}

func (driver *Driver) AddNodeServiceCapabilities(capability []csi.NodeServiceCapability_RPC_Type) {
	nsc := []*csi.NodeServiceCapability{}
	for _, n := range capability {
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	driver.nscap = nsc
}

func NewControllerServiceCapability(capability csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: capability,
			},
		},
	}
}

func NewGroupControllerServiceCapability(capability csi.GroupControllerServiceCapability_RPC_Type) *csi.GroupControllerServiceCapability {
	return &csi.GroupControllerServiceCapability{
		Type: &csi.GroupControllerServiceCapability_Rpc{
			Rpc: &csi.GroupControllerServiceCapability_RPC{
				Type: capability,
			},
		},
	}
}

func NewNodeServiceCapability(capability csi.NodeServiceCapability_RPC_Type) *csi.NodeServiceCapability {
	return &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: capability,
			},
		},
	}
}
