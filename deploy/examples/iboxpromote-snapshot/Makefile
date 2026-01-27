include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

.PHONY: setup-snapshot
setup-snapshot: devEnv teardown-snapshot
	@echo -e $(_begin)
	envsubst < ./iboxpromote-snapshot.yaml | $(_CLI) create -f -
	@echo -e $(_finish)


.PHONY: teardown-snapshot
teardown-snapshot: devEnv 
	@echo -e $(_begin)
	envsubst < ./iboxpromote-snapshot.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

