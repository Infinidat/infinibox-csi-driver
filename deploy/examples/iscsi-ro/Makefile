include ../../../Makefile-help

##@ a readonly iscsi example
.PHONY: setup
setup:  devEnv teardown
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc.yaml
	$(_CLIN) create -f ./pod-writer.yaml
	sleep 20
	$(_CLIN) create -f ./pod.yaml
	@echo -e $(_finish)


.PHONY: teardown
teardown: devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-writer.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)


