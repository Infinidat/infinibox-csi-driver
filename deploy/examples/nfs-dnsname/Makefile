include ../Makefile-vars  # User configured vars
include ../../../Makefile-help

_purpose = This nfs example tests the dnsname parameter that can be specified in a storageclass, to run this test you will have to create dnsname that will resolve on the kubernetes hosts (e.g. /etc/hosts) that resolves to an ip address on your ibox NAS network space

##@ NFS Complete Example
.PHONY: setup
setup:  teardown ## Deploy NFS objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_clinn) create -f ./storageclass.yaml
	$(_cli) create -f ./pvc.yaml
	$(_cli) create -f ./app.yaml
	@echo -e $(_finish)


.PHONY: teardown
teardown:  ## Remove everything including K3s.
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./app.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./pvc.yaml
	$(_clinn) delete --wait --ignore-not-found=true -f ./storageclass.yaml
	@echo -e $(_finish)

.PHONY: setup-clone
setup-clone:  teardown-clone ## Deploy NFS objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_cli) create -f ./clonepvc.yaml
	$(_cli) create -f ./appclone.yaml
	@echo -e $(_finish)

.PHONY: teardown-clone
teardown-clone:  ## Remove clone resources
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./appclone.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./clonepvc.yaml
	@echo -e $(_finish)

.PHONY: setup-snapshot
setup-snapshot:  teardown-snapshot ## Deploy NFS objects, restore snapshot, clone.
	@echo -e $(_begin)
	$(_cli) create -f ./snapshot.yaml
	sleep 5
	$(_cli) create -f ./restoresnapshot.yaml
	sleep 5
	$(_cli) create -f ./apprestoresnapshot.yaml
	@echo -e $(_finish)

.PHONY: teardown-snapshot
teardown-snapshot:  ## Remove snapshot resources
	@echo -e $(_begin)
	$(_cli) delete --wait --ignore-not-found=true -f ./apprestoresnapshot.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./restoresnapshot.yaml
	$(_cli) delete --wait --ignore-not-found=true -f ./snapshot.yaml
	@echo -e $(_finish)
