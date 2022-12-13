_USE_COLOR ?= true

include Makefile-vars  # User configured vars
include Makefile-logging
include Makefile-secrets
# include Makefile-testing-upstream-e2e
include ../../Makefile-help

SHELL := bash

_purpose = The infindat-csi-driver-demo project demonstrates basic deployment and operatation of our CSI driver using a K3s cluster. Makefiles automate each step showing how this may be accomplished. See The README for details.

_helm_chart_src_dir = ../helm/infinibox-csi-driver
# _helm_chart_snapshot_controller_src_dir = ./charts/snapshot-controller

##@ K3s
.PHONY: setup-k3s
setup-k3s:  ## Install k3s.
	@echo -e $(_begin)
	@./k3s.sh
	@sudo chmod 666 "/etc/rancher/k3s/k3s.yaml"
	@$(_make) setup-snapshot-controller
	@echo -e "\nNote: In every shell running k3s kubectl, run "
	@echo -e "\t'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml'\n"
	@echo    "or add to .bashrc"
	@echo -e $(_finish)

.PHONY: setup-k3s-installer
setup-k3s-installer:  ## Install k3s with specified version using k3s_install.sh.
	@echo -e $(_begin)
	K3S_KUBECONFIG_MODE="644" INSTALL_K3S_VERSION="$(_k3s_ver)" ./k3s_install.sh
	@echo -e $(_finish)

.PHONY: teardown-k3s
teardown-k3s: teardown-csi  ## Remove k3s.
	@echo -e $(_begin)
	@k3s --version &> /dev/null && \
	/usr/local/bin/k3s-uninstall.sh	|| \
	echo "K3s not installed"
	@echo -e $(_finish)

##@ snapshot-controller (Use only if not deployed by default by CO)
.PHONY: setup-snapshot-controller
setup-snapshot-controller:  ## Install the snapshot-controller as a Helm release.
	@echo -e $(_begin)
	if [ "$(_enable_snapshot_controller_chart)" == "true" ]; then \
		$(_clinn) create ns $(_namespace_snapshot_controller) 2>&1 | egrep "created|already exists" && \
		cd "./charts/snapshot-controller" && \
			helm -n "$(_namespace_snapshot_controller)" install "$(_helm_release_snapshot_controller)" . 2>&1 \
				| egrep 'deployed|cannot re-use a name that is still in use' && \
		cd -; \
	else \
		echo "Snapshot-controller's helm chart is not enabled. See Makefile-vars."; \
		false; \
	fi
	@echo -e $(_finish)

.PHONY: teardown-snapshot-controller
teardown-snapshot-controller:  ## Uninstall the snapshot-controller Helm release.
	@echo -e $(_begin)
	@helm -n $(_namespace_snapshot_controller) uninstall "$(_helm_release_snapshot_controller)" 2>&1 | egrep "uninstalled|not found"
	@echo "Removing CSI Driver custom resource definitions (not removed by helm uninstall)"
	@echo "Error 'not found' is ignored"
	@declare -a crds=("volumesnapshotclasses.snapshot.storage.k8s.io" "volumesnapshotcontents.snapshot.storage.k8s.io" "volumesnapshots.snapshot.storage.k8s.io"); \
	for crd in "$${crds[@]}"; do \
		kubectl delete crd $$crd 2>&1 | egrep "deleted|not found"; \
	done
	@#$(_make) teardown-secrets
	@echo -e $(_finish)

##@ CSI
.PHONY: validate-chart-values
validate-chart-values:  ## Validate that the helm chart's values.yaml has been configured.
	@# Check values have been configured in the chart.
	@echo -e $(_begin)
	@echo "Recipe temporarily defeated"
	@# @declare -a tools=("spruce" "jq"); \
	# for tool in "$${tools[@]}"; do \
	# 	if $$($$tool --version 2>&1 > /dev/null); then \
	# 		: \
	# 	else \
	# 		echo "$$tool unavailable. Cannot validate helm chart values.yaml"; \
	# 		exit 0; \
	# 	fi \
	# done
	@# @# Fail if ibox0000 is the configured ibox hostname in values.yaml.
	@# @# Use spruce to convert to json. Use jq to filter value.
	@# @declare -r invalid_ibox="ibox0000"; \
	# if [[ $$(spruce json "$(_helm_chart_src_dir)/values.yaml" | jq '.Infinibox_Cred.hostname') =~ "$$invalid_ibox" ]]; then \
	# 	echo "Helm chart values.yaml needs to be configured"; \
	# 	echo "Infinibox_Cred.hostname cannot be $$invalid_ibox"; \
	# 	exit 2; \
	# fi
	@# @echo "Helm chart's values.yaml is minimally validated"
	@echo -e $(_finish)

.PHONY: setup-csi
setup-csi: validate-chart-values  ## Install the CSI Driver as a Helm release.
	@echo -e $(_begin)
	@$(_clinn) create ns $(_namespace) 2>&1 | egrep "created|already exists"
	@$(_make) create-secrets
	@cd "$(_helm_chart_src_dir)" && \
		$(_helm) install "$(_helm_release)" . \
		2>&1 | egrep "deployed|cannot re-use a name that is still in use"&& \
	cd -
	@echo -e $(_finish)

.PHONY: upgrade-csi
upgrade-csi: validate-chart-values  ## Upgrade CSI Driver Helm release named csi-infinibox.
	@echo -e $(_begin)
	@$(_clinn) create ns $(_namespace) 2>&1 | egrep "created|already exists"
	@$(_make) create-secrets
	@cd $(_helm_chart_src_dir) && \
		$(_helm) upgrade "$(_helm_release)" . && \
	cd -
	@echo -e $(_finish)

.PHONY: teardown-csi
teardown-csi: _teardown-dependencies  ## Uninstall the CSI Driver Helm release.
	@echo -e $(_begin)
	$(_helm) uninstall "$(_helm_release)" 2>&1 | egrep "uninstalled|not found"
	@echo "Removing CSI Driver custom resource definitions (not removed by helm uninstall)"
	@echo "Error 'not found' is ignored"
	$(_make) teardown-secrets
	@echo -e $(_finish)

.PHONY: _teardown-dependencies
_teardown-dependencies:
	@# Call teardown-nfs in nfs/, iscsi/, fc/ and treeq/.
	@cd nfs && \
		$(_make) teardown-nfs
	cd - \
	# cd iscsi && \
	# 	$(_make) teardown-iscsi
	# cd -
	@$(_make) teardown-snapshot-controller
