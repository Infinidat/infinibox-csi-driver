
include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ fc examples
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./statefulset.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  devEnv 
	@echo -e $(_begin)
	envsubst < ./statefulset.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

