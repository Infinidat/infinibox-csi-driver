include ../../../Makefile-help

##@ iscsi block rwx example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc-block-rwx.yaml
	$(_CLIN) create -f './pod-block-*.yaml'
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv 
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f './pod-block-*.yaml'
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-block-rwx.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)
