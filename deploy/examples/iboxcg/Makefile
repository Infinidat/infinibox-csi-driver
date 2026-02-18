include ../../../Makefile-vars-git-ignored
export

include ../../../Makefile-help

.PHONY: setup-add
setup-add: devEnv teardown-add
	@echo -e $(_begin)
	envsubst < ./iboxcg-add.yaml | $(_CLI) create -f -
	@echo -e $(_finish)


.PHONY: teardown-add
teardown-add: devEnv 
	@echo -e $(_begin)
	envsubst < ./iboxcg-add.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

.PHONY: setup-remove
setup-remove: devEnv teardown-remove
	@echo -e $(_begin)
	envsubst < ./iboxcg-remove.yaml | $(_CLI) create -f -
	@echo -e $(_finish)

.PHONY: teardown-remove
teardown-remove: devEnv 
	@echo -e $(_begin)
	envsubst < ./iboxcg-remove.yaml | $(_CLI) delete --wait --ignore-not-found=true -f -
	@echo -e $(_finish)

