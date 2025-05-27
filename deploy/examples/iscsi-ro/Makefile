include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

##@ a readonly iscsi example
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pod-writer.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-writer.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)


.PHONY: setup-reader
setup-reader:  devEnv teardown-reader
	@echo -e $(_begin)
	envsubst < ./pv-ro.yaml | $(_CLI) create -f -
	envsubst < ./pvc-ro.yaml | $(_CLIN) create -f -
	envsubst < ./pod-ro.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-reader
teardown-reader: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-ro.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pvc-ro.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	envsubst < ./pv-ro.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

