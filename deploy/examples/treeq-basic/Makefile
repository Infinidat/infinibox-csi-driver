include ../../../Makefile-help

##@ treeq Complete Example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc.yaml
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-fsgroup
setup-fsgroup:  devEnv teardown-fsgroup
	@echo -e $(_begin)
	envsubst < ./storageclass-fsgroup.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc-fsgroup.yaml
	envsubst < ./pod-fsgroup.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown-fsgroup
teardown-fsgroup: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-fsgroup.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-fsgroup.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass-fsgroup.yaml
	@echo -e $(_finish)
