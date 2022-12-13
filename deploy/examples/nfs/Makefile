include ../Makefile-vars  # User configured vars
include ../../../Makefile-help

_purpose = The infindat-csi-driver-demo project demonstrates basic deployment and operatation of our CSI driver using a K3s cluster. Makefiles automate each step showing how this may be accomplished. See The README for details.

##@ NFS Complete Example
.PHONY: setup-nfs
setup-nfs:  ## Deploy NFS objects, restore snapshot, clone.
	@echo -e $(_begin)
	@$(_cli) create ns $(_namespace) 2>&1 | egrep "created|already exists"
	@$(_make) create-nfs-objects
	@$(_make) restore-from-snap-nfs
	@$(_make) clone-pvc-nfs
	@$(_make) clone-snapshot-nfs
	@echo -e $(_finish)

.PHONY: teardown-nfs
teardown-nfs:  ## Remove everything including K3s.
	@echo -e $(_begin)
	@# $(_make) remove-clone-pvc-nfs
	@$(_make) remove-clone-snapshot-nfs
	@$(_make) remove-clone-pvc-nfs
	@$(_make) remove-restore-from-snap-nfs
	@$(_make) teardown-nfs-objects
	@echo -e $(_finish)

##@ NFS
.PHONY: create-nfs-objects
create-nfs-objects:  ## Create all nfs objects including a snap.
	@echo -e $(_begin)
	@echo "Errors such as 'already exists' are ignored"
	declare -a yamls=( \
		"storageclass.yaml" \
		"pv.yaml" \
		"pvc.yaml" \
		"snapshotclass.yaml" \
		"snapshot.yaml" \
		"app.yaml" \
	); \
	for yaml in "$${yamls[@]}"; do \
		$(_cli) create -f "$$yaml" 2>&1 | egrep "already exists|created"; \
	done
	@echo -e $(_finish)

.PHONY: teardown-nfs-objects
teardown-nfs-objects: remove-clone-pvc-nfs remove-restore-from-snap-nfs  ## Remove all nfs objects including a snap.
	@echo -e $(_begin)
	@echo "Errors such as 'not found' are ignored"
	@declare -a   kinds=("pod"               "volumesnapshot"         "volumesnapshotclass"     "pvc"           "pod"                ) && \
	declare  -a objects=("ibox-pod-pvc-demo" "ibox-pvc-snapshot-demo" "ibox-snapshotclass-demo" "ibox-pvc-demo" "ibox-pod-pvc-demo" )&& \
	for i in "$${!objects[@]}"; do \
		echo "Deleting kind [$${kinds[i]}] named [$${objects[i]}]" && \
		$(_cli) delete "$${kinds[i]}" "$${objects[i]}" 2>&1 | egrep "deleted|not found|NotFound|doesn.t have a resource type" > /dev/null \
			|| exit 1; \
	done || exit 1
	@echo "Deleting storageclass after short sleep to allow deletiong of pvc to complete"
	@sleep 3 && \
	declare -a kinds=(  "storageclass"              ) && \
	declare -a objects=("ibox-nfs-storageclass-demo") && \
	for i in "$${!objects[@]}"; do \
		echo "Deleting kind [$${kinds[i]}] named [$${objects[i]}]" && \
		$(_cli) delete "$${kinds[i]}" "$${objects[i]}" 2>&1 | egrep "deleted|not found|NotFound" > /dev/null \
			|| exit 1; \
	done || exit 1
	@echo -e $(_finish)

.PHONY: restore-from-snap-nfs
restore-from-snap-nfs:  ## Restore a nfs pv from a snapshot.
	@echo -e $(_begin)
	@echo "Errors such as 'already exists' are ignored"
	@declare -a yamls=("restoresnapshot.yaml"); \
	for yaml in "$${yamls[@]}"; do \
		$(_cli) create -f "$$yaml" 2>&1 | egrep "already exists|created"; \
	done
	@echo -e $(_finish)

.PHONY: remove-restore-from-snap-nfs
remove-restore-from-snap-nfs:  ## Remove nfs pvc used to restore a pv from snapshot.
	@echo -e $(_begin)
	@echo "Errors such as 'not found' are ignored"
	@declare -a kinds=(  "pvc"                             ); \
	declare  -a objects=("ibox-snapshot-pvc-restore-demo-2"); \
	for i in "$${!objects[@]}"; do \
		echo "Deleting kind [$${kinds[i]}] named [$${objects[i]}]"; \
		$(_cli) delete "$${kinds[i]}" "$${objects[i]}" 2>&1 | grep --extended-regexp "deleted|not found" > /dev/null \
			|| exit 1; \
	done || exit 1
	@echo -e $(_finish)

.PHONY: clone-pvc-nfs
clone-pvc-nfs:  ## Clone an nfs pvc.
	@echo -e $(_begin)
	@echo "Errors such as 'already exists' are ignored"
	@declare -a yamls=("clonepvc.yaml"); \
	for yaml in "$${yamls[@]}"; do \
		$(_cli) create -f "$$yaml" 2>&1 | egrep "already exists|created"; \
	done
	@echo -e $(_finish)

.PHONY: remove-clone-pvc-nfs
remove-clone-pvc-nfs:  ## Remove nfs cloned pvc.
	@echo -e $(_begin)
	@echo "Errors such as 'not found' are ignored"
	@declare -a kinds=(  "pvc"                 "volumesnapshot"); \
	declare  -a objects=("ibox-pvc-clone-demo" "ibox-pvc-clone-snapshot-demo"); \
	for i in "$${!objects[@]}"; do \
		echo "Deleting kind [$${kinds[i]}] named [$${objects[i]}]"; \
		$(_cli) delete "$${kinds[i]}" "$${objects[i]}" 2>&1 | grep --extended-regexp "deleted|not found" > /dev/null \
			|| exit 1; \
	done || exit 1
	@echo -e $(_finish)

.PHONY: clone-snapshot-nfs
clone-snapshot-nfs:  ## Clone an nfs snapshot.
	@echo -e $(_begin)
	@echo "Errors such as 'already exists' are ignored"
	@declare -a yamls=("clonesnapshot.yaml"); \
	for yaml in "$${yamls[@]}"; do \
		$(_cli) create -f "$$yaml" 2>&1 | egrep "already exists|created"; \
	done
	@echo -e $(_finish)

.PHONY: remove-clone-snapshot-nfs
remove-clone-snapshot-nfs:  ## Remove nfs snapshot clone.
	@echo -e $(_begin)
	@echo "Errors such as 'not found' are ignored"
	@declare -a kinds=(  "VolumeSnapshot"              ); \
	declare  -a objects=("ibox-pvc-clone-snapshot-demo"); \
	for i in "$${!objects[@]}"; do \
		echo "Deleting kind [$${kinds[i]}] named [$${objects[i]}]"; \
		$(_cli) delete "$${kinds[i]}" "$${objects[i]}" 2>&1 | grep --extended-regexp "deleted|not found" > /dev/null \
			|| exit 1; \
	done || exit 1
	@echo -e $(_finish)
