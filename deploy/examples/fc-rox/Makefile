include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ fc rox example
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

.PHONY: setup-reader
setup-reader:  devEnv teardown-reader
	@echo -e $(_begin)
	$(_CLIN) create -f ./pvc-rox.yaml
	envsubst < ./pod-rox.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-reader
teardown-reader:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod-rox.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-rox.yaml
	@echo -e $(_finish)

.PHONY: setup-rox-pv
setup-rox-pv:  devEnv
	@echo -e $(_begin)
	$(_CLI) create -f ./pv.yaml
	@echo -e $(_finish)

.PHONY: teardown-rox-pv
teardown-rox-pv:  devEnv
	@echo -e $(_begin)
	envsubst < ./pv.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
