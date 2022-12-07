# Use of examples

Purpose: Provide customers with examples for using Infinidat's CSI driver demonstrating basic use cases.  This follows the examples from our CSI User Guide. To facilitate this a Makefile is provided with recipes for use cases.  Before use cases can be explored, a Kubernetes cluster is required and our CSI driver needs to be installed.  The Makefile provides recipes for setting up a kubernetes cluster test cluster using K3s. If you have a cluster of your own, you certainly may use that instead of K3s. These resources are not meant for use in production environments.

If one does not wish to use the Makefile recipes, they still serve as readable working examples of how to proceed. Each recipe is just a label on top of some bash.  Many recipes have dependencies that execute first, especially for teardowns.

`Make help` will show all recipes available in the current directory. Protocol directories have their own Makefiles with protocol-specific reciples.

Note that this README lists setup make recipes. Each has an equivalent teardown recipe. To see these, run `make help`.

## examples/
The Make recipes in examples/ provide an optional quick setup for creating a kubernetes cluster using Rancher's K3s. This is provided for convenience. If you **have an existing cluster**, use that instead.

Some kubernetes distributions, such as K3s, do not deploy a snapshot controller by default. Again for convenience, we provide a snapshot controller helm chart along with make recipes to deploy it. If no snapshot controller is available in your cluster, snapshotting and cloning in the examples will fail.

Infinidat's CSI driver is helm based. Make recipes are provided to install and uninstall our CSI driver.

cd to examples and run `make help` to see these and other recipes:
- setup-k3s
- setup-snapshot-controller
- setup-csi

There are several convenience recipes such as:
- watch-ns
- follow-node-driver-log
- follow-driver-driver-log
- infinishell-events

## nfs/
The nfs/ examples demonstrate using NFS to create PVCs dynamically using an NFS storageclass and using this in a pod.  Snapshot and clone examples are also shown.

cd to examples/nfs and run `make help` to see these and other recipes:
- setup-nfs

## iscsi/
- TODO

## fc/
- TODO

## treeq/
- TODO
