include ../../../Makefile-help

##@ iscsi multiple network spaces example
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc.yaml
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-clone
setup-clone:  devEnv teardown-clone
	@echo -e $(_begin)
	$(_CLIN) create -f ./pvc-clone.yaml
	envsubst < ./pod-clone.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-clone
teardown-clone:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod-clone.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-clone.yaml
	@echo -e $(_finish)

.PHONY: setup-snapshot
setup-snapshot:  devEnv teardown-snapshot
	@echo -e $(_begin)
	$(_CLI) create -f ../volumesnapshotclass.yaml
	$(_CLIN) create -f ./snapshot.yaml
	sleep 5
	$(_CLIN) create -f ./pvc-restore-snapshot.yaml
	sleep 5
	envsubst < ./pod-restore-snapshot.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-snapshot
teardown-snapshot:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod-restore-snapshot.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-restore-snapshot.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./snapshot.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ../volumesnapshotclass.yaml
	@echo -e $(_finish)

