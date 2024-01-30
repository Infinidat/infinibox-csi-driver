include ../../Makefile-vars  # User configured vars
include ../../../../Makefile-help

_purpose = This example shows how treeq can be pre-provisioned by an ibox admin, then used by kube admins.

##@ treeq admin Example
.PHONY: setup
setup:  teardown ## Deploy treeq objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass.yaml
	$(_cli) create -f ./user1-pv.yaml
	$(_cli) create -f ./user2-pv.yaml
	$(_cli) create -f ./user1-pvc.yaml
	$(_cli) create -f ./user2-pvc.yaml
	$(_cli) create -f ./user1-app.yaml
	$(_cli) create -f ./user2-app.yaml
	@echo -e $(_finish)


.PHONY: teardown
teardown:  ## Remove everything including K3s.
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./user1-app.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./user2-app.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./user1-pvc.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./user2-pvc.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./user1-pv.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./user2-pv.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)
