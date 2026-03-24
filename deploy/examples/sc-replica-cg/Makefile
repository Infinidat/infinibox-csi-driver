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

.PHONY: teardown
COMMAND_OUTPUT := $(shell kubectl get pvc sc-replica-cg -o jsonpath='{.spec.volumeName}')
teardown:  devEnv
	@echo -e $(_begin)
	kubectl delete iboxcg iboxcg-$$COMMAND_OUTPUT --ignore-not-found=true
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

