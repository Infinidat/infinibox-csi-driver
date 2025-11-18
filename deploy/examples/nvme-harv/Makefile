include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ nvme Complete Example
.PHONY: setup
setup:  devEnv teardown 
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-snapshot
setup-snapshot:  devEnv
	@echo -e $(_begin)
	envsubst < ../volumesnapshotclass.yaml | $(_CLI) create -f -
	@echo -e $(_finish)

