kubectl delete -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n infi
kubectl delete -f infinibox-operator/deploy/operator.yaml -n infi
kubectl delete -f infinibox-operator/deploy/role_binding.yaml -n infi
kubectl delete -f infinibox-operator/deploy/role.yaml -n infi
kubectl delete -f infinibox-operator/deploy/service_account.yaml -n infi
kubectl delete -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n infi