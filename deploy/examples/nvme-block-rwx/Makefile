include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAME=nvme-block-rwx

##@ nvme block rwx example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod-node1.yaml | $(_CLIN) create -f -
	envsubst < ./pod-node2.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv 
	@echo -e $(_begin)
	envsubst < ./pod-node1.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod-node2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
