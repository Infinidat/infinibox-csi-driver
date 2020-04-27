# Overview
Helm Chart for InfiniBox CSI driver deployment

# Usage
## Install driver
 - Modify values.yaml to include Infinibox hostname and Pool Admin credentials
 - Create a name space for CSI driver deployment
   kubectl create namespace infinibox
 - Install the driver
   helm install csi-infinibox -n=infinibox ./

## Uninstall driver
   helm uninstall csi-infinibox -n=infinibox


