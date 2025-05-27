include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ fc rox example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-reader
setup-reader:  devEnv teardown-reader
	@echo -e $(_begin)
	envsubst < ./pvc-rox.yaml | $(_CLIN) create -f -
	envsubst < ./pod-rox.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-reader
teardown-reader:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod-rox.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-rox.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-rox-pv
setup-rox-pv:  devEnv
	@echo -e $(_begin)
	envsubst < ./pv.yaml | $(_CLI) create -f -
	@echo -e $(_finish)

.PHONY: teardown-rox-pv
teardown-rox-pv:  devEnv
	@echo -e $(_begin)
	envsubst < ./pv.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
