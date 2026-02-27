include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ iscsi sc replica example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: setup-remote
setup-remote:  devEnv teardown-remote
	@echo -e $(_begin)
	envsubst < ./pod-remote.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-remote
teardown-remote:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod-remote.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv teardown-remote
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

