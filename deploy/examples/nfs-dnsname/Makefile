include ../../../Makefile-help

##@ example of specifying dnsname in storage class 
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

.PHONY: setup-clone
setup-clone: devEnv teardown-clone 
	@echo -e $(_begin)
	envsubst < ./pvc-clone.yaml | $(_CLIN) create -f -
	envsubst < ./pod-clone.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-clone
teardown-clone: devEnv 
	@echo -e $(_begin)
	envsubst < ./pod-clone.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-clone.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-snapshot
setup-snapshot:  devEnv teardown-snapshot
	@echo -e $(_begin)
	envsubst < ../volumesnapshotclass.yaml | $(_CLI) create -f -
	sleep 5
	envsubst < ./snapshot.yaml | $(_CLIN) create -f -
	sleep 5
	envsubst < ./pvc-restore-snapshot.yaml | $(_CLIN) create -f -
	sleep 5
	envsubst < ./pod-restore-snapshot.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-snapshot
teardown-snapshot: devEnv 
	@echo -e $(_begin)
	envsubst < ./pod-restore-snapshot.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-restore-snapshot.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./snapshot.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ../volumesnapshotclass.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
