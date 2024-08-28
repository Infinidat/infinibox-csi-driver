include ../../../Makefile-help

##@ iscsi example using pvc annotations
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-fsgroup
setup-fsgroup: devEnv  teardown-fsgroup ## Deploy iscsi objects
	@echo -e $(_begin)
	envsubst < ./storageclass-fsg.yaml | $(_CLI) create -f -
	envsubst < ./pvc-fsg.yaml | $(_CLIN) create -f -
	envsubst < ./pod-fsg.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-fsgroup
teardown-fsgroup: devEnv  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-fsg.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-fsg.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass-fsg.yaml
	@echo -e $(_finish)

.PHONY: setup-clone
setup-clone: devEnv  teardown-clone
	@echo -e $(_begin)
	envsubst < ./pvc-clone.yaml | $(_CLIN) create -f -
	envsubst < ./pod-clone.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-clone
teardown-clone: devEnv 
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-clone.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-clone.yaml
	@echo -e $(_finish)

.PHONY: setup-snapshot
setup-snapshot: devEnv teardown-snapshot 
	@echo -e $(_begin)
	$(_CLIN) create -f ../volumesnapshotclass.yaml
	$(_CLIN) create -f ./snapshot.yaml
	sleep 5
	$(_CLIN) create -f ./restore-snapshot.yaml
	sleep 5
	envsubst < ./pod-restore-snapshot.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-snapshot
teardown-snapshot: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-restore-snapshot.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./restore-snapshot.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./snapshot.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ../volumesnapshotclass.yaml
	@echo -e $(_finish)

.PHONY: setup-block
setup-block: devEnv teardown-block
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc-block.yaml | $(_CLIN) create -f -
	enbsubst < ./pod-block.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-block
teardown-block: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-block.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-block.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-block-clone
setup-block-clone: devEnv teardown-block-clone
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc-block-clone.yaml | $(_CLIN) create -f -
	envsubst < ./pod-block-clone.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-block-clone
teardown-block-clone: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-block-clone.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-block-clone.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

