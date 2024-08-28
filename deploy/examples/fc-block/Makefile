include ../../../Makefile-help

##@ fc block example
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
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-block-clone
setup-block-clone:  devEnv teardown-block-clone
	@echo -e $(_begin)
	$(_CLI) create -f ./storageclass.yaml
	$(_CLIN) create -f ./pvc-block-clone.yaml
	envsubst < ./pod-block-clone.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-block-clone
teardown-block-clone:  devEnv 
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-block-clone.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-block-clone.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)
