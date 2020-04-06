oc delete -f infinibox-operator/deploy/crds/infinibox-csi-driver-service.yaml -n infi
oc delete -f infinibox-operator/deploy/operator.yaml -n infi
oc delete -f infinibox-operator/deploy/role_binding.yaml -n infi
oc delete -f infinibox-operator/deploy/role.yaml -n infi
oc delete -f infinibox-operator/deploy/service_account.yaml -n infi
oc delete -f infinibox-operator/deploy/crds/infiniboxcsidriver_crd.yaml -n infi