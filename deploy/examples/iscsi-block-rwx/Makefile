include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAME=iscsi-block-rwx

##@ iscsi block rwx example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc-block-rwx.yaml | $(_CLIN) create -f -
	envsubst < ./pod-block-node1.yaml | $(_CLIN) create -f -
	envsubst < ./pod-block-node2.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv 
	@echo -e $(_begin)
	envsubst < ./pod-block-node1.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod-block-node2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod-block-rwx.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
