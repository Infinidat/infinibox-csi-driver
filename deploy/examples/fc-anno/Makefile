include ../../../Makefile-help

##@ fc with pvc annotations example
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc.yaml
	$(_CLIN) create -f ./pod.yaml
	@echo -e $(_finish)

.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-fsgroup
setup-fsgroup:  devEnv teardown-fsgroup
	@echo -e $(_begin)
	envsubst < ./storageclass-fsg.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc-fsg.yaml
	$(_CLIN) create -f ./pod-fsg.yaml
	@echo -e $(_finish)

.PHONY: teardown-fsgroup
teardown-fsgroup:  devEnv 
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-fsg.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-fsg.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass-fsg.yaml
	@echo -e $(_finish)

.PHONY: setup-clone
setup-clone:  devEnv teardown-clone
	@echo -e $(_begin)
	$(_CLIN) create -f ./pvc-clone.yaml
	$(_CLIN) create -f ./pod-clone.yaml
	@echo -e $(_finish)

.PHONY: teardown-clone
teardown-clone: devEnv 
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-clone.yaml
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
	$(_CLIN) create -f ./pod-restore-snapshot.yaml
	@echo -e $(_finish)

.PHONY: teardown-snapshot
teardown-snapshot:  devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-restore-snapshot.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-restore-snapshot.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./snapshot.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ../volumesnapshotclass.yaml
	@echo -e $(_finish)
