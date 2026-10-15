include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ iscsi Complete Example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ../volumesnapshotclass.yaml | $(_CLI) create -f -
	sleep 10
	oc annotate --overwrite storageprofile ibox-iscsi-default  cdi.kubevirt.io/minimumSupportedPvcSize=1Gi
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	envsubst < ../volumesnapshotclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

