include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ This example shows how treeq can be pre-provisioned by an ibox admin, then used by kube admins.
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pv-user1.yaml | $(_CLI) create -f -
	envsubst < ./pv-user2.yaml | $(_CLI) create -f -
	envsubst < ./pvc-user1.yaml | $(_CLIN) create -f -
	envsubst < ./pvc-user2.yaml | $(_CLIN) create -f -
	envsubst < ./pod-user1.yaml | $(_CLIN) create -f -
	envsubst < ./pod-user2.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-user1.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pod-user2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-user2.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-user1.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pv-user1.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	envsubst < ./pv-user2.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
