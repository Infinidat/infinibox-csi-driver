include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAME=iscsi-2-pods

##@ iscsi Complete Example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ../pvc.yaml | $(_CLIN) create -f -
	envsubst < ../pod.yaml | $(_CLIN) create -f -
	envsubst < ./pvc2.yaml | $(_CLIN) create -f -
	envsubst < ./pod2.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	envsubst < ../pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ../pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

