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
	"infinibox-csi-driver/helper"
	"runtime"

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

	//ids *identityServer
	ns          *NodeServer
	cscap       []*csi.ControllerServiceCapability
	nscap       []*csi.NodeServiceCapability
	volumeLocks *helper.VolumeLocks
}

func NewDriver(options *DriverOptions) *Driver {
	zlog.Info().Msgf("Driver: %v version: %v", options.DriverName, options.Version)

	n := &Driver{
		name:             options.DriverName,
		version:          options.Version,
		nodeID:           options.NodeID,
		endpoint:         options.Endpoint,
		mountPermissions: options.MountPermissions,
		workingMountDir:  options.WorkingMountDir,
	}

	n.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		/**
		currently unimplemented
		*/
		//csi.ControllerServiceCapability_RPC_GET_CAPACITY
		//csi.ControllerServiceCapability_RPC_PUBLISH_READONLY
		//csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES
		//csi.ControllerServiceCapability_RPC_VOLUME_CONDITION
		//csi.ControllerServiceCapability_RPC_GET_VOLUME
		//csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER

	})

	n.AddNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_UNKNOWN,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,

		/**
		currently unimplemented
		*/
		//csi.NodeServiceCapability_RPC_GET_VOLUME_STATS
		//csi.NodeServiceCapability_RPC_VOLUME_CONDITION
		//csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER
		//csi.NodeServiceCapability_RPC_VOLUME_MOUNT_GROUP
	})
	n.volumeLocks = helper.NewVolumeLocks()
	return n
}

func NewNodeServer(n *Driver, mounter mount.Interface) *NodeServer {
	return &NodeServer{
		Driver:  n,
		mounter: mounter,
	}
}

func (n *Driver) Run(testMode bool) {

	mounter := mount.New("")
	if runtime.GOOS == "linux" {
		// MounterForceUnmounter is only implemented on Linux now
		mounter = mounter.(mount.MounterForceUnmounter)
	}
	n.ns = NewNodeServer(n, mounter)
	s := NewNonBlockingGRPCServer()
	s.Start(n.endpoint,
		NewDefaultIdentityServer(n),
		NewControllerServer(n),
		n.ns,
		testMode)
	s.Wait()
}

func NewDefaultIdentityServer(d *Driver) *IdentityServer {
	return &IdentityServer{
		Driver: d,
	}
}

func NewControllerServer(d *Driver) *ControllerServer {
	return &ControllerServer{
		Driver: d,
	}
}

func (n *Driver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability
	for _, c := range cl {
		csc = append(csc, NewControllerServiceCapability(c))
	}
	n.cscap = csc
}

func (n *Driver) AddNodeServiceCapabilities(nl []csi.NodeServiceCapability_RPC_Type) {
	var nsc []*csi.NodeServiceCapability
	for _, n := range nl {
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	n.nscap = nsc
}

func NewControllerServiceCapability(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func NewNodeServiceCapability(cap csi.NodeServiceCapability_RPC_Type) *csi.NodeServiceCapability {
	return &csi.NodeServiceCapability{
		Type: &csi.NodeServiceCapability_Rpc{
			Rpc: &csi.NodeServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}
