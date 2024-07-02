include ../../../Makefile-help

##@ NFS Complete Example
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
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml --force
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLINnn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-clone
setup-clone: devEnv  teardown-clone 
	@echo -e $(_begin)
	$(_CLIN) create -f ./pvc-clone.yaml
	$(_CLIN) create -f ./pod-clone.yaml
	@echo -e $(_finish)


.PHONY: teardown-clone
teardown-clone: devEnv  
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-clone.yaml --force
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-clone.yaml
	@echo -e $(_finish)
