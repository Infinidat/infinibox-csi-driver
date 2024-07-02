include ../../../Makefile-help

##@ an example of 2 pods using the same RWX volume claim
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc.yaml
	$(_CLIN) create -f ./pod.yaml
	$(_CLIN) create -f ./pod-2.yaml
	@echo -e $(_finish)


.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml --force
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-2.yaml  --force
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

