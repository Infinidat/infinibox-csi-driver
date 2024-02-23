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

.PHONY: setup-fsgroup
setup-fsgroup:  teardown-fsgroup ## Deploy iscsi objects
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass-fsg.yaml
	$(_cli) create -f ./pvc-fsg.yaml
	$(_cli) create -f ./app-fsg.yaml
	@echo -e $(_finish)

.PHONY: teardown-fsgroup
teardown-fsgroup:  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app-fsg.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc-fsg.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass-fsg.yaml
	@echo -e $(_finish)



.PHONY: setup-clone
setup-clone:  teardown-clone ## Deploy iscsi objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_cli) create -f ./clonepvc.yaml
	$(_cli) create -f ./appclone.yaml
	@echo -e $(_finish)

.PHONY: teardown-clone
teardown-clone:  ## Remove clone resources
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./appclone.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./clonepvc.yaml
	@echo -e $(_finish)

.PHONY: setup-snapshot
setup-snapshot:  teardown-snapshot ## Deploy iscsi objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_cli) create -f ../volumesnapshotclass.yaml
	sleep 5
	$(_cli) create -f ./snapshot.yaml
	sleep 5
	$(_cli) create -f ./restoresnapshot.yaml
	sleep 5
	$(_cli) create -f ./apprestoresnapshot.yaml
	@echo -e $(_finish)

.PHONY: teardown-snapshot
teardown-snapshot:  ## Remove snapshot resources
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./apprestoresnapshot.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./restoresnapshot.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./snapshot.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ../volumesnapshotclass.yaml
	@echo -e $(_finish)

.PHONY: setup-block
setup-block:  teardown-block ## Deploy iscsi objects
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass.yaml
	$(_cli) create -f ./pvc-block.yaml
	$(_cli) create -f ./app-block.yaml
	@echo -e $(_finish)

.PHONY: teardown-block
teardown-block:  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app-block.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc-block.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-block-clone
setup-block-clone:  teardown-block-clone ## Deploy iscsi objects
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass.yaml
	$(_cli) create -f ./pvc-block-clone.yaml
	$(_cli) create -f ./app-block-clone.yaml
	@echo -e $(_finish)

.PHONY: teardown-block-clone
teardown-block-clone:  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app-block-clone.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc-block-clone.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

