include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ iscsi sc replica example
.PHONY: setup
setup:  devEnv teardown teardown-remote
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: setup-remote
setup-remote:  devEnv 
	@echo -e $(_begin)
	envsubst < ./pod-remote.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-remote
teardown-remote:  devEnv
	@echo -e $(_begin)
	envsubst < ./pod-remote.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete pvc sc-replica-remote-pvc$(_E2E_IBOXREPLICA_REMOTE_PVC_NAME_SUFFIX) --wait --ignore-not-found=true

.PHONY: teardown
	COMMAND_OUTPUT := $(shell kubectl get pvc sc-replica-remote-pvc -o jsonpath='{.spec.volumeName}')
teardown:  devEnv teardown-remote
	@echo -e $(_begin)
	kubectl delete iboxreplica iboxreplica-$$COMMAND_OUTPUT --ignore-not-found=true
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	#$(_CLI) delete pv $(COMMAND_OUTPUT)$(_E2E_IBOXREPLICA_REMOTE_PVC_NAME_SUFFIX) --wait --ignore-not-found=true
	@echo -e $(_finish)

