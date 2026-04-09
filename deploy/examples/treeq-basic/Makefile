include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

TEST_NAMe=treeq-basic

##@ treeq Complete Example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ../pvc.yaml | $(_CLIN) create -f -
	envsubst < ../pod.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	envsubst < ../pod.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ../pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-fsgroup
setup-fsgroup:  devEnv teardown-fsgroup
	@echo -e $(_begin)
	envsubst < ./storageclass-fsgroup.yaml | $(_CLI) create -f -
	envsubst < ./pvc-fsgroup.yaml | $(_CLIN) create -f -
	envsubst < ./pod-fsgroup.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown-fsgroup
teardown-fsgroup: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-fsgroup.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-fsgroup.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass-fsgroup.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
