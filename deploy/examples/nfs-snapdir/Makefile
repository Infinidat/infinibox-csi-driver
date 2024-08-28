include ../../../Makefile-help

_purpose = an example of nfs specifying snapdirvisible parameter

.PHONY: setup-fsgroup
setup-fsgroup:  devEnv teardown-fsgroup
	@echo -e $(_begin)
	envsubst < ./storageclass-fsgroup.yaml | $(_CLI) create -f -
	$(_CLIN) create -f ./pvc-fsgroup.yaml
	envsubst < ./pod-fsgroup.yaml | $(_CLIN) create -f -
	@echo -e $(_finish)


.PHONY: teardown-fsgroup
teardown-fsgroup:  devEnv
	@echo -e $(_begin)
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pod-fsgroup.yaml
	$(_CLIN) delete --wait --ignore-not-found=true -f ./pvc-fsgroup.yaml
	$(_CLI) delete --wait --ignore-not-found=true -f ./storageclass-fsgroup.yaml
	@echo -e $(_finish)

