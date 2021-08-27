# Overview
This is a Helm chart to deploy the InfiniBox CSI driver. See more details and requirements in the [InfiniBox CSI driver user guide](https://support.infinidat.com/hc/en-us/articles/360008917097-InfiniBox-CSI-Driver-for-Kubernetes-User-Guide).

# Usage
## Install driver
 - Modify `values.yaml` to include InfiniBox hostname, Pool Admin credentials, and Kubernetes secret name
 - Create a namespace for CSI driver deployment - e.g. `infi`
   `kubectl create namespace infi`
 - Install the driver
   `helm install csi-infinibox -n=infi ./`

## Uninstall driver
   `helm uninstall csi-infinibox -n=infi`
