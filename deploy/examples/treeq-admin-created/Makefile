include ../../../Makefile-help

##@ This example shows how treeq can be pre-provisioned by an ibox admin, then used by kube admins.
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLI) create -f ./pv-user1.yaml
	$(_CLI) create -f ./pv-user2.yaml
	$(_CLIN) create -f ./pvc-user1.yaml
	$(_CLIN) create -f ./pvc-user2.yaml
	envsubst < ./pod-user1.yaml | $(_CLIN) create -f -
	envsubst < ./pod-user2.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-user1.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod-user2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-user1.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-user2.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./pv-user1.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./pv-user2.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)
