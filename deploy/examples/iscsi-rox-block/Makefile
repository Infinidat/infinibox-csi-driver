include ../../../Makefile-help

##@ iscsi rox block example
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

.PHONY: setup-rox
setup-rox:  devEnv teardown-rox
	@echo -e $(_begin)
	envsubst < ./storageclass-rox.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pv-rox.yaml
	$(_CLIN) create -f ./pvc-rox.yaml
	$(_CLIN) create -f ./pod-rox.yaml
	@echo -e $(_finish)

.PHONY: teardown-rox
teardown-rox: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-rox.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-rox.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pv-rox.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass-rox.yaml
	@echo -e $(_finish)

