include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAME=iscsi-block-test

##@ iscsi block example
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

.PHONY: setup-block-clone
setup-block-clone:  devEnv teardown-block-clone
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc-block-clone.yaml | $(_CLIN) create -f -
	envsubst < ./pod-block-clone.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-block-clone
teardown-block-clone: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-block-clone.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-block-clone.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

