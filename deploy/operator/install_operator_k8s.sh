#! /bin/bash

# set name space for operator here
nmspace="infi"

kubectl get ns | grep $nmspace 
if [ $? -ne 0 ];
  then echo "creating namespace"; kubectl create ns $nmspace
fi


kubectl create -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n $nmspace
kubectl create -f infinibox-operator/deploy/service_account.yaml -n $nmspace
kubectl create -f infinibox-operator/deploy/role.yaml -n $nmspace
kubectl create -f infinibox-operator/deploy/role_binding.yaml -n $nmspace
kubectl create -f infinibox-operator/deploy/operator.yaml -n $nmspace
kubectl create -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n $nmspace
