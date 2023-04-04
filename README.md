# Overview
  This is the official [Container Storage Interface (CSI)](https://kubernetes-csi.github.io/docs/) Driver for Infinidat InfiniBox storage systems. For more details, review the [user guide](https://support.infinidat.com/hc/en-us/articles/360008917097-InfiniBox-CSI-Driver-for-Kubernetes-User-Guide).

# Prerequisites
  Infinidat [Host PowerTools](https://repo.infinidat.com/home/main-stable#host-power-tools) is recommended to validate connectivity and host best practices.

## Supported container environments
  - Kubernetes 1.23 - 1.26
  - Helm version 3.1.0 and above
  - Red Hat OpenShift 4.9 - 4.12

## Platform requirements
  - Latest Linux multipath software package for your operating system
  - Latest filesystem utilities/drivers (XFS, etc.)
  - Latest iSCSI initiator software (for iSCSI connectivity)
  - Latest Fibre Channel initiator software for your operating system (for FC connectivity)
  - Virtualized environments must use pass-through mode for Fibre Channel connectivity
  - Latest NFS software package for your operating system (for NFS / NFS TreeQs)
 
# Installation
  Helm and Operator based installation is available. Refer to the InfiniBox CSI Driver [user guide](https://support.infinidat.com/hc/en-us/articles/360008917097-InfiniBox-CSI-Driver-for-Kubernetes-User-Guide) for details.

# Support
   Certain CSI features may be in alpha or beta status and such features should not be used for production environments; Refer to the [official CSI feature gate table](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/) and [InfiniBox CSI Driver release notes](https://support.infinidat.com/hc/en-us/articles/360019909678-InfiniBox-CSI-Driver-for-Kubernetes-Release-Notes) for details.

# License
  This is open source software licensed under the Apache License 2.0. See LICENSE file in this repository for details.
