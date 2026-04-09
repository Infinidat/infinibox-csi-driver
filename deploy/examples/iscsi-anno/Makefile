include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAME=iscsi-anno-test

##@ iscsi example using pvc annotations
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ../pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ../pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-fsgroup
setup-fsgroup: devEnv  teardown-fsgroup ## Deploy iscsi objects
	@echo -e $(_begin)
	envsubst < ./storageclass-fsg.yaml | $(_CLI) create -f -
	envsubst < ./pvc-fsg.yaml | $(_CLIN) create -f -
	envsubst < ./pod-fsg.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-fsgroup
teardown-fsgroup: devEnv  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	envsubst < ./pod-fsg.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-fsg.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass-fsg.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-clone
setup-clone: devEnv  teardown-clone
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
setup-snapshot: devEnv teardown-snapshot 
	@echo -e $(_begin)
	envsubst < ../volumesnapshotclass.yaml | $(_CLI) create -f -
	envsubst < ../snapshot.yaml | $(_CLIN) create -f -
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
	envsubst < ../snapshot.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ../volumesnapshotclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-block
setup-block: devEnv teardown-block
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc-block.yaml | $(_CLIN) create -f -
	envsubst < ./pod-block.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-block
teardown-block: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-block.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-block.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-block-clone
setup-block-clone: devEnv teardown-block-clone
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
