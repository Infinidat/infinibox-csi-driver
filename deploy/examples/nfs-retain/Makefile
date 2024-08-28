include ../../../Makefile-help

##@ NFS example of using Retain
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pv.yaml
	$(_CLIN) create -f ./pvc.yaml
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pv.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)
