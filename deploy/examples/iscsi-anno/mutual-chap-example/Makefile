include ../../Makefile-vars  # User configured vars
include ../../../../Makefile-help

_purpose = this is an iscsi mutual chap example

##@ iscsi Complete Example
.PHONY: setup
setup:  teardown ## Deploy iscsi objects
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_clinn) create -f -
	$(_cli) create -f ./pvc.yaml
	envsubst < ./app.yaml | $(_cli) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

