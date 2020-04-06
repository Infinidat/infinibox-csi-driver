#! /bin/bash
oc get ns | grep infi 
if [ $? -ne 0 ];
  then echo "creating namespace"; oc create ns infi
fi


oc create -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n infi
oc create -f infinibox-operator/deploy/service_account.yaml -n infi
oc create -f infinibox-operator/deploy/role.yaml -n infi
oc create -f infinibox-operator/deploy/role_binding.yaml -n infi
oc create -f infinibox-operator/deploy/operator.yaml -n infi
oc create -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n infi