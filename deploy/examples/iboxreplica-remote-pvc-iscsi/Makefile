include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ iscsi example using iboxreplica-sync volume with create_pvc 
.PHONY: setup-pod
setup-pod:  devEnv 
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) create -f -
.PHONY: teardown-pod
teardown-pod: devEnv
	@echo -e $(_begin)
	envsubst < ./pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
.PHONY: teardown-iboxreplica
teardown-iboxreplica: devEnv
	@echo -e $(_begin)
	envsubst < ./iboxreplica.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -

.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./iboxreplica.yaml | $(_CLI) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./iboxreplica.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete pvc/iboxreplica-remote-pvc-iscsi --wait --ignore-not-found=true 
	$(_CLI) delete pv/iboxreplica-remote-pvc-iscsi --wait --ignore-not-found=true 
	@echo -e $(_finish)
