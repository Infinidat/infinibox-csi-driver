# Overview
This is a Helm chart to deploy the InfiniBox COSI driver. See more details and requirements in the [InfiniBox CSI driver user guide](https://support.infinidat.com/hc/en-us/articles/10106070174749-InfiniBox-CSI-Driver-for-Kubernetes-User-Guide).

# Usage
## Install driver
 - Modify `values.yaml` to include InfiniBox hostname, Pool Admin credentials, and Kubernetes secret name
 - Install the driver
   `helm install infinidat-cosi-driver .`

## Uninstall driver
   `helm uninstall infinidat-cosi-driver`
