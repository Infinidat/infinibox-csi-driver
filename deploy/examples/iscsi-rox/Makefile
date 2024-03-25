include ../Makefile-vars  # User configured vars
include ../../../Makefile-help

_purpose = The infindat-csi-driver-demo project demonstrates basic deployment and operatation of our CSI driver using a K3s cluster. Makefiles automate each step showing how this may be accomplished. See The README for details.

##@ iscsi Complete Example
.PHONY: setup
setup:  teardown ## Deploy iscsi objects
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass.yaml
	$(_cli) create -f ./pvc.yaml
	$(_cli) create -f ./app.yaml
	@echo -e $(_finish)

.PHONY: teardown
teardown:  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

