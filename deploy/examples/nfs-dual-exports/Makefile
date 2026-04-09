include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ NFS ip-range Example.. 
##@ the ip range here is invalid, but the example should run
##@ because the node ip is evaluated against the range and if
##@ not within the range, the node ip is automatically added
##@ into the export permissions

TEST_NAME=nfs-dual-exports

.PHONY: setup
setup: devEnv teardown 
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
