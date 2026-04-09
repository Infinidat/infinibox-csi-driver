include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAME=iscsi-rox-block

##@ iscsi rox block example
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-rox
setup-rox:  devEnv teardown-rox
	@echo -e $(_begin)
	envsubst < ./storageclass-rox.yaml | $(_CLI) create -f -
	envsubst < ./pv-rox.yaml | $(_CLI) create -f -
	envsubst < ./pvc-rox.yaml | $(_CLIN) create -f -
	envsubst < ./pod-rox.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-rox
teardown-rox: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-rox.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-rox.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pv-rox.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass-rox.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

