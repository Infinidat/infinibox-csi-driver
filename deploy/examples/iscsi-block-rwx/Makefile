include ../../../Makefile-help

##@ iscsi block rwx example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc-block-rwx.yaml
	envsubst < ./pod-block-node1.yaml | $(_CLIN) create -f -
	envsubst < ./pod-block-node2.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv 
	@echo -e $(_begin)
	envsubst < ./pod-block-node1.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod-block-node2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-block-rwx.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)
