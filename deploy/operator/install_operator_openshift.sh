#! /bin/bash

# set user here
opnusr="system:admin"
# set name space for operator here
opnmspace="infi"

oc get ns | grep $opnmspace 
if [ $? -ne 0 ];
  then echo "creating namespace"; oc create ns $opnmspace
fi

sed -i 's/namespace=.*/namespace=${opnmspace}/' infinibox-operator/deploy/role_binding.yaml

oc create -f scc/iboxcsiaccess_scc.yaml --as $opnusr
oc adm policy add-scc-to-user iboxcsiaccess -z infinibox-csi-driver-node -n $opnmspace
oc adm policy add-scc-to-user iboxcsiaccess -z infinibox-csi-driver-driver -n $opnmspace
oc create -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n $opnmspace
oc create -f infinibox-operator/deploy/service_account.yaml -n $opnmspace
oc create -f infinibox-operator/deploy/role.yaml -n $opnmspace
oc create -f infinibox-operator/deploy/role_binding.yaml -n $opnmspace
oc create -f infinibox-operator/deploy/operator.yaml -n $opnmspace
oc create -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n $opnmspace