include ../../Makefile-vars  # User configured vars
include ../../../../Makefile-help

_purpose = this is an iscsi mutual chap example

##@ iscsi Complete Example
.PHONY: setup
setup:  teardown ## Deploy iscsi objects
	@echo -e $(_begin)
	envsubst < ./storageclass.yaml | $(_CLI) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./pvc.yaml | $(_CLIN) create -f -
	envsubst < ./app.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)

.PHONY: teardown
teardown:  ## Remove resources from setup-iscsi
	@echo -e $(_begin)
	envsubst < ./app.yaml | $(_CLIN) delete -f -
	envsubst < ./pvc.yaml | $(_CLIN) delete -f -
	envsubst < ./storageclass.yaml | $(_CLI) delete -f -
	@echo -e $(_finish)

