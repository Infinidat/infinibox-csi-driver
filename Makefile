# vim: set foldmethod=indent foldnestmax=1 foldcolumn=1:

EUID := $(shell id -u -r)
ifneq ($(EUID),0)
	_SUDO = sudo
endif

# CICD defines _GITLAB_USER and other vars, and so CICD should not to include Makefile-vars-git-ignored.
# See Makefile-vars-git-ignored for and example. This file is not checked in by design.
ifneq ($(_GITLAB_USER),gitlab-user-name)
	include Makefile-vars-git-ignored
endif
include Makefile-help
include Makefile-git
include Makefile-e2e
include Makefile-release
include Makefile-metrics

_GOCMD              ?= $(shell which go)

# Go parameters.
# Timestamp go binary. See var compileDate in main.go.
_GOBUILD            = $(_GOCMD) build -ldflags "-X main.compileDate=$$(date -u +%Y-%m-%d_%H:%M:%S_%Z) -X main.gitHash=$$(git rev-parse HEAD) -X main.version=$(_IMAGE_TAG) -X main.goVersion='$$(go version | sed 's/ /_/g')"
_GOCLEAN            = $(_GOCMD) clean
_GOTEST             = $(_GOCMD) test
_GOMOD              = $(_GOCMD) mod
_GOFMT              = gofmt
_GOLINT             = golangci-lint

_LOCALDIR ?= $(shell pwd)

# Docker image tag. Read from env or use default
_IMAGE_TAG   ?= v2.7.1

_GITLAB_REPO        = git.infinidat.com:4567
_BINARY_NAME        = infinibox-csi-driver
_DRIVER_IMAGE   = infinidat-csi-driver
_art_dir            = artifact

# For Development Build #################################################################
# Docker.io username and tag
_DOCKER_USER        = infinidat
_DOCKER_BASE_IMAGE  = redhat/ubi8:latest

# For Production Build ##################################################################
ifeq ($(env),prod)
	_IMAGE_TAG=$(_IMAGE_TAG)
	# For Production
	# Do not change following values unless change in production version or username
	# For docker.io
	_DOCKER_USER=infinidat
	_IMAGE_TAG=$(_IMAGE_TAG)
endif
# For Production Build ##################################################################

##@ Go

.PHONY: clean
clean:  ## Clean source.
	$(_GOCLEAN)
	rm -f $(_BINARY_NAME)

.PHONY: build
build:  ## Build source.
	@echo -e $(_begin)
	$(_GOBUILD) -o $(_BINARY_NAME) -v
	@echo -e $(_finish)

.PHONY: rebuild
rebuild: clean ## Rebuild source (all packages)
	$(_GOBUILD) -o $(_BINARY_NAME) -v -a

.PHONY: test
test: build  ## Unit test source.
	@echo -e $(_begin)
	$(_GOTEST) -v ./... -tags unit
	@echo -e $(_finish)

.PHONY: test-one-thing
test-one-thing: build lint  ## Unit test source, but just run one test.
	@echo -e $(_begin)
	@printf "\nFrom $(_TEST_ONE_THING_DIR)/, running test $(_TEST_ONE_THING)\n\n"
	@sleep 1
	@cd "$(_TEST_ONE_THING_DIR)" && \
		$(_GOTEST) -v -run "$(_TEST_ONE_THING)" \
		&&  printf "\nTest passed = From $(_TEST_ONE_THING_DIR)/, ran test $(_TEST_ONE_THING)\n\n" \
		|| (printf "\nTest failed - From $(_TEST_ONE_THING_DIR)/, ran test $(_TEST_ONE_THING)\n\n"; false)
	@echo -e $(_finish)

.PHONY: test-find-fails
test-find-fails:  ## Find and summarize failing tests.
	@echo -e $(_begin)
	@$(_make) test | grep "    --- FAIL:"
	@echo -e $(_finish)

.PHONY: lint
lint: build  ## Lint source.
	@echo -e $(_begin)
	@$(_GOLINT) run
	@echo -e $(_finish)

.PHONY: fmt
fmt: build  ## Format and simplify source.
	@echo -e $(_begin)
	$(_GOFMT) -s -w .
	@echo -e $(_finish)

.PHONY: modverify
modverify:  ## Verify dependencies have expected content.
	$(_GOMOD) verify

.PHONY: modtidy
modtidy:  ## Add missing and remove unused modules.
	$(_GOMOD) tidy

.PHONY: moddownload
moddownload:  ## Download modules to local cache.
	$(_GOMOD) download

##@ Cross compilation
.PHONY: build-linux
build-linux:  ## Cross compile CSI driver for Linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(_GOBUILD) -o $(_BINARY_NAME) -v

##@ Docker
.PHONY: image
image: build lint test  ## Build and tag CSI driver docker image.
	@echo -e $(_begin)
	@echo "Pulling base image $(_DOCKER_BASE_IMAGE)"
	@docker pull $(_DOCKER_BASE_IMAGE)
	@export HEAD=$$(git rev-parse --short HEAD); \
	export BLAME_MACHINE=$$(hostname); \
	export BLAME_USER=$${USER}; \
	export BLAME_BUILD_TIME="$$(date)"; \
	echo "Building CSI driver image [$(_IMAGE_TAG)] from commit [$$HEAD] at [$$BLAME_BUILD_TIME]"; \
	docker build $(OPTIONAL_DOCKER_BUILD_FLAGS) -t "$(_DOCKER_USER)/$(_DRIVER_IMAGE):$(_IMAGE_TAG)" \
		--build-arg IMAGE_TAG="$(_IMAGE_TAG)" \
		--build-arg VCS_REF="$$HEAD" \
		--build-arg BLAME_MACHINE="$$BLAME_MACHINE" \
		--build-arg BLAME_USER="$$BLAME_USER" \
		--build-arg BLAME_BUILD_TIME="$$BLAME_BUILD_TIME" \
		--pull \
		-f Dockerfile .
	@# TODO tag cmd needs review.
	docker tag $(_DOCKER_USER)/$(_DRIVER_IMAGE):$(_IMAGE_TAG) $(_GITLAB_USER)/infinidat-csi-driver/$(_DRIVER_IMAGE):$(_IMAGE_TAG)
	@echo -e $(_finish)

.PHONY: docker-login-docker
docker-login-docker:  ## Login to Dockerhub.
	@docker login

.PHONY: image-push 
image-push:  ## Tag and push CSI driver image to gitlab.
	$(eval _TARGET_IMAGE=$(_GITLAB_REPO)/$(_GITLAB_USER)/infinidat-csi-driver/$(_DRIVER_IMAGE):$(_IMAGE_TAG))
	docker tag $(_GITLAB_USER)/infinidat-csi-driver/$(_DRIVER_IMAGE):$(_IMAGE_TAG) $(_TARGET_IMAGE)
	docker push $(_GITLAB_REPO)/$(_GITLAB_USER)/infinidat-csi-driver/$(_DRIVER_IMAGE):$(_IMAGE_TAG)

.PHONY: docker-push-redhat
docker-push-redhat:  ## Login, tag and push to Red Hat.
	@# Ref: https://connect.redhat.com/projects/5e9f4fa0ebed1415210b4b24/images/upload-image
	@echo "The password is a token acquired by https://connect.redhat.com/projects/5e9f4fa0ebed1415210b4b24/images/upload-image"
	docker login -u unused scan.connect.redhat.com
	docker tag $(_GITLAB_REPO)/$(_GITLAB_USER)/$(_DRIVER_IMAGE):$(_IMAGE_TAG) scan.connect.redhat.com/ospid-956ccd64-1dcf-4d00-ba98-336497448906/$(_DRIVER_IMAGE):$(_IMAGE_TAG)
	docker push scan.connect.redhat.com/ospid-956ccd64-1dcf-4d00-ba98-336497448906/$(_DRIVER_IMAGE):$(_IMAGE_TAG)

.PHONY: docker-push-all
docker-push-all: image-push docker-push-redhat docker-push-dockerhub  ## Push to both Gitlab, Red Hat and Dockerhub.

.PHONY: buildlocal
buildlocal: build docker-build clean

.PHONY: all
all: build docker-build docker-push clean

.PHONY: docker-image-save
docker-image-save: ## Save image to gzipped tar file to _art_dir.
	mkdir -p $(_art_dir) && \
	docker save $(_DOCKER_USER)/$(_DRIVER_IMAGE):$(_IMAGE_TAG) \
		| gzip > ./$(_art_dir)/$(_DRIVER_IMAGE)_$(_IMAGE_TAG)_docker-image.tar.gz

.PHONY: docker-helm-chart-save
docker-helm-chart-save:  ## Save the helm chart to a tarball in _art_dir.
	mkdir -p $(_art_dir) && \
	tar cvfz ./$(_art_dir)/$(_DOCKER_IMAGE)_$(_IMAGE_TAG)_helm-chart.tar.gz deploy/helm
	@# --exclude='*.un~'

.PHONY: docker-save
docker-save: docker-image-save docker-helm-chart-save ## Save the image and the helm chart to the _art_dir so that they may be provided to others.

.PHONY: docker-load-help
docker-load-help:  ## Show a hint for how to load a docker image.
	@echo "docker load < <docker image tar file>"

.PHONY: docker-rmi-dangling
docker-rmi-dangling:  ## Remove docker images that are dangling to recover disk space.
	@echo -e $(_begin)
	docker rmi $$(docker images -q -f dangling=true)
	@echo -e $(_finish)

.PHONY: docker-push-host-opensource
docker-push-host-opensource:  ## Push CSI driver images to host-opensource.
	@echo -e $(_begin)
	docker tag git.infinidat.com:4567/$(_GITLAB_USER)/infinidat-csi-driver/infinidat-csi-driver-controller:$(_IMAGE_TAG) \
		git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-driver-controller:$(_IMAGE_TAG)
	docker push git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-driver-controller:$(_IMAGE_TAG)
	docker tag git.infinidat.com:4567/$(_GITLAB_USER)/infinidat-csi-driver/infinidat-csi-driver-node:$(_IMAGE_TAG) \
		git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-driver-node:$(_IMAGE_TAG)
	docker push git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-driver-node:$(_IMAGE_TAG)
	@echo -e $(_finish)

.PHONY: docker-push-dockerhub
docker-push-dockerhub: docker-login-docker  ## Push host-opensource CSI driver images to dockerhub.
	@echo -e $(_begin)
	docker tag git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-driver-controller:$(_IMAGE_TAG) \
		infinidat/infinidat-csi-driver-controller:$(_IMAGE_TAG)
	docker push infinidat/infinidat-csi-driver-controller:$(_IMAGE_TAG)
	docker tag git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-driver-node:$(_IMAGE_TAG) \
		infinidat/infinidat-csi-driver-node:$(_IMAGE_TAG)
	docker push infinidat/infinidat-csi-driver-node:$(_IMAGE_TAG)
	@echo -e $(_finish)

.PHONY: github-push
_GIT_REMOTE ?= git@github.com:Infinidat/infinibox-csi-driver.git
_GIT_PUSH_OPTIONS ?=
github-push:  ## Push develop to Github with optional git push options.
	@echo -e $(_begin)
	git push $(_GIT_PUSH_OPTIONS) "$(_GIT_REMOTE)" develop:develop
	@echo -e $(_finish)

.PHONY: version
version:  ## Show tool versions.
	@echo -e $(_begin)
	@$(_GOCMD) version
	@echo "_IMAGE_TAG: $(_IMAGE_TAG)"
	@echo -e $(_finish)

# Force the _check-make-vars-defined recipe to always run. Verify our make variables have been defined.
# While Makefile will not usually have changed, its prerequisite will have to run regardless.
# Do not use .PHONY on this Makefile rule.
Makefile: _check-make-vars-defined

.PHONY: _check-make-vars-defined
_check-make-vars-defined:
	@#Verify our make variables have been defined.
ifndef _GITLAB_USER
	$(error _GITLAB_USER is not set)
endif
ifndef _TEST_ONE_THING_DIR
	$(error _TEST_ONE_THING_DIR is not set)
endif
ifndef _TEST_ONE_THING
	$(error _TEST_ONE_THING is not set)
endif
