# Overview
  This is the official [Container Storage Interface (CSI)](https://kubernetes-csi.github.io/docs/) driver for Infinidat InfiniBox storage systems. For more details, review the [user guide](https://support.infinidat.com/hc/en-us/articles/360008917097-InfiniBox-CSI-Driver-for-Kubernetes-User-Guide).

# Prerequisites
  Infinidat [Host PowerTools](https://repo.infinidat.com/home/main-stable#host-power-tools) is recommended to validate connectivity and host best practices.

## Supported containers environments
  - Kubernetes 1.20-1.23
  - Minimum Helm version required is 3.1.0
  - Red Hat OpenShift 4.6-4.10

## Platform requirements
  - Latest Linux multipath software package for your operating system
  - Latest filesystem utilities/drivers (XFS, etc)
  - Latest iSCSI initiator software (for iSCSI connectivity)
  - Latest Fibre Channel initiator software for your operating system (for FC connectivity)
  - Virtualized environments must use pass-through mode for Fibre Channel connectivity
  - Latest NFS software package for your operating system (for NFS / NFS TreeQs)
 
# Installation
  Helm and Operator based installation is available. See InfiniBox CSI driver [user guide](https://support.infinidat.com/hc/en-us/articles/360000633265) for details.

# Support
  Infinidat provides comprehensive enterprise-grade support for Infinidat storage in containers environments. See [Infinidat support web site](https://support.infinidat.com) for details.
  Certain CSI features may be in alpha or beta status and such features should not be used for production environments; see [official CSI feature gate table](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/) and [InfiniBox CSI driver release notes](https://support.infinidat.com/hc/en-us/articles/360019909678-InfiniBox-CSI-Driver-for-Kubernetes-Release-Notes) for details.

# License
  This is open source software licensed under the Apache License 2.0. See (LICENSE)[LICENSE] for details.
