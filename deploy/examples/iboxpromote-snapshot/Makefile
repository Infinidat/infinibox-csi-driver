include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

.PHONY: setup-volume
setup-volume: devEnv teardown-volume
	@echo -e $(_begin)
	envsubst < ./iboxreplica-async-volume.yaml | $(_CLI) create -f -
	@echo -e $(_finish)


.PHONY: teardown-volume
teardown-volume: devEnv 
	@echo -e $(_begin)
	envsubst < ./iboxreplica-async-volume.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-filesystem
setup-filesystem: devEnv teardown-filesystem
	@echo -e $(_begin)
	envsubst < ./iboxreplica-async-filesystem.yaml | $(_CLI) create -f -
	@echo -e $(_finish)


.PHONY: teardown-filesystem
teardown-filesystem: devEnv 
	@echo -e $(_begin)
	envsubst < ./iboxreplica-async-filesystem.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-cg
setup-cg: devEnv teardown-cg
	@echo -e $(_begin)
	envsubst < ./iboxreplica-async-cg.yaml | $(_CLI) create -f -
	@echo -e $(_finish)


.PHONY: teardown-cg
teardown-cg: devEnv 
	@echo -e $(_begin)
	envsubst < ./iboxreplica-async-cg.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)
