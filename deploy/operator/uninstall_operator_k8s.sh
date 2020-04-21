#! /bin/bash

nmspace="infi"

kubectl delete -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n $nmspace
kubectl delete -f infinibox-operator/deploy/operator.yaml -n $nmspace
kubectl delete -f infinibox-operator/deploy/role_binding.yaml -n $nmspace
kubectl delete -f infinibox-operator/deploy/role.yaml -n $nmspace
kubectl delete -f infinibox-operator/deploy/service_account.yaml -n $nmspace
kubectl delete -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n $nmspace
