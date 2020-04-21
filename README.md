**Overview**
  
  This project implements the InfiniBox CSI Driver.

**Platform and Software dependencies**
  **Operating Systems Supported:**
  - CentOS 7,8
  - RHEL 7, 8
  - Ubuntu 16.04, 18.04
      
  **Environments Supported:**
  - Kubernetes 1.14+
  - Minimum Helm version required is 3.1.0.
  - Pivotal Kubernetes Service 1.5, 1.6
  - RedHat OpenShift 4.x

**Other software dependencies:**
  **For iSCSI and FC:**
  - Latest linux multipath software package for your operating system
  - Latest Filesystem utilities/drivers (XFS, etc)
  - Latest iSCSI initiator software (for iSCSI connectivity)
  - Latest FC initiator software for your operating system (for FC connectivity. FC is supported on Bare-metal or on VM in pass-through mode)
  - Optional: Host Power Tools to ensure proper iSCSI/FC configuration

  **For NFS and NFS-Treeq:** 
  - Latest NFS software package for your operating system
 
**Installation details:**
   - Follow Infinibox CSI driver user guide

 
