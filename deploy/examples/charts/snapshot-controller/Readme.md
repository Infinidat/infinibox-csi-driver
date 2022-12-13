# Overview
This is a Helm chart to deploy the snapshot controller. Not all kubernetes distributions deploy a snapshot controller by default (e.g. K3s and Local Openshift, formally Codeready Container (crc)). Without a snapshot controller, snapshot will work as expected.  This was tested only on K3s.  This chart is not appropriate for a CRC.

# Usage
## Install snapshot controller
 - Modify `values.yaml` to include an imagePullSecret if required
 - Install the snapshot controller
   `helm -n infi install snap-release ./`

## Uninstall snapshot controller
   `helm -n infi uninstall snap-release`
