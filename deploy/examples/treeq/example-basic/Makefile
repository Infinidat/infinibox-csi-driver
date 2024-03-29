include ../../Makefile-vars  # User configured vars
include ../../../../Makefile-help

_purpose = The infindat-csi-driver-demo project demonstrates basic deployment and operatation of our CSI driver using a K3s cluster. Makefiles automate each step showing how this may be accomplished. See The README for details.

##@ treeq Complete Example
.PHONY: setup
setup:  teardown ## Deploy NFS objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass.yaml
	$(_cli) create -f ./pvc.yaml
	$(_cli) create -f ./app.yaml
	@echo -e $(_finish)


.PHONY: teardown
teardown:  ## Remove everything including K3s.
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-fsgroup
setup-fsgroup:  teardown-fsgroup ## Deploy NFS objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass-fsg.yaml
	$(_cli) create -f ./pvc-fsg.yaml
	$(_cli) create -f ./app-fsg.yaml
	@echo -e $(_finish)


.PHONY: teardown-fsgroup
teardown-fsgroup:  ## Remove everything including K3s.
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app-fsg.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc-fsg.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass-fsg.yaml
	@echo -e $(_finish)