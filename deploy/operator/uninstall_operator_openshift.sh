#! /bin/bash

opnmspace="infi"

oc delete -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n $opnmspace
oc delete -f infinibox-operator/deploy/operator.yaml -n $opnmspace
oc delete -f infinibox-operator/deploy/role_binding.yaml -n $opnmspace
oc delete -f infinibox-operator/deploy/role.yaml -n $opnmspace
oc delete -f infinibox-operator/deploy/service_account.yaml -n $opnmspace
oc delete -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n $opnmspace
