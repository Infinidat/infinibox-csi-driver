include ../../../Makefile-help

##@ a readonly iscsi example
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc.yaml
	envsubst < ./pod-writer.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-writer.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)


.PHONY: setup-reader
setup-reader:  devEnv teardown-reader
	@echo -e $(_begin)
	$(_CLIN) create -f ./pv-ro.yaml
	$(_CLIN) create -f ./pvc-ro.yaml
	envsubst < ./pod-ro.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown-reader
teardown-reader: devEnv
	@echo -e $(_begin)
	envsubst < ./pod-ro.yaml | $(_CLIN) delete --wait --ignore-not-found=true -f -
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-ro.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./pv-ro.yaml
	@echo -e $(_finish)

