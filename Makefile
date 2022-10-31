# vim: set foldmethod=indent foldnestmax=1 foldcolumn=1:
SHELL = /bin/bash

EUID := $(shell id -u -r)
ifneq ($(EUID),0)
	_SUDO = sudo
endif

include Makefile-help
include Makefile-git

_GOCMD              ?= $(shell which go)

# Go parameters.
# Timestamp go binary. See var compileDate in main.go.
_DOCKER_IMAGE_TAG   = v2.2.0-rc2
_GOBUILD            = $(_GOCMD) build -ldflags "-X main.compileDate=$$(date --utc +%Y-%m-%d_%H:%M:%S_%Z) -X main.gitHash=$$(git rev-parse HEAD) -X main.version=$(_DOCKER_IMAGE_TAG) -X main.goVersion='$$(go version | sed 's/ /_/g')"
_GOCLEAN            = $(_GOCMD) clean
_GOTEST             = $(_SUDO) $(_GOCMD) test
_GOMOD              = $(_GOCMD) mod
_GOFMT              = gofumpt
_GOLINT             = golangci-lint

_REDHAT_REPO        = scan.connect.redhat.com
_GITLAB_REPO        = git.infinidat.com:4567
_BINARY_NAME        = infinibox-csi-driver
_DOCKER_IMAGE       = infinidat-csi-driver
_art_dir            = artifact

# For Development Build #################################################################
# Docker.io username and tag
_DOCKER_USER        = infinidat
_GITLAB_USER        = dohlemacher
_DOCKER_BASE_IMAGE  = redhat/ubi8:latest

# redhat username and tag
_REDHAT_DOCKER_USER = dohlemacher2
_REDHAT_DOCKER_IMAGE_TAG = $(_DOCKER_IMAGE_TAG)

# For Production Build ##################################################################
ifeq ($(env),prod)
	_IMAGE_TAG=$(_DOCKER_IMAGE_TAG)
	# For Production
	# Do not change following values unless change in production version or username
	# For docker.io
	_DOCKER_USER=infinidat
	_DOCKER_IMAGE_TAG=$(_IMAGE_TAG)

	# For scan.connect.redhat.com
	_REDHAT_DOCKER_USER=ospid-956ccd64-1dcf-4d00-ba98-336497448906
	_REDHAT_DOCKER_IMAGE_TAG=$(_IMAGE_TAG)
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
	$(_GOTEST) -v ./...
	@echo -e $(_finish)

.PHONY: test-one-thing
test-one-thing: build lint  ## Unit test source, but just run one test.
	@echo -e $(_begin)
	@export testdir=storage; \
	export test=TestNodeSuite/Test_updateNfsMountOptions_badNfsVersion; \
	printf "\nFrom $$testdir, running test $$test\n\n"; \
	cd "$$testdir" && \
	$(_GOTEST) -v -run "$$test"
	@echo -e $(_finish)

.PHONY: test-find-fails
test-find-fails:  ## Find and summarize failing tests.
	@echo -e $(_begin)
	@$(_make) test | grep "    --- FAIL:"
	@echo -e $(_finish)

.PHONY: lint
lint: build ## Lint source.
	@echo -e $(_begin)
	$(_GOLINT) run
	@echo -e $(_finish)

.PHONY: fmt
fmt: build ## Auto-format source
	@echo -e $(_begin)
	$(_GOFMT) -w -l .
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
.PHONY: docker-build-docker
docker-build-docker: build lint test  ## Build and tag CSI driver docker image.
	@echo -e $(_begin)
	@echo "Pulling base image $(_DOCKER_BASE_IMAGE)"
	@docker pull $(_DOCKER_BASE_IMAGE)
	@export HEAD=$$(git rev-parse --short HEAD); \
	export BLAME_MACHINE=$$(hostname); \
	export BLAME_USER=$${USER}; \
	echo "Building CSI driver image [$(_DOCKER_IMAGE_TAG)] from commit [$$HEAD]"; \
	docker build -t $(_DOCKER_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG) \
		--build-arg DOCKER_IMAGE_TAG=$(_DOCKER_IMAGE_TAG) \
		--build-arg VCS_REF=$$HEAD \
		--build-arg BLAME_MACHINE=$$BLAME_MACHINE \
		--build-arg BLAME_USER=$$BLAME_USER \
		-f Dockerfile .
	@# TODO tag cmd needs review.
	docker tag $(_DOCKER_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG) $(_GITLAB_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG)
	@echo -e $(_finish)

.PHONY: docker-build-redhat
docker-build-redhat: build test  ## Build and tag CSI driver for Red Hat docker repo.
	docker build -t $(_REDHAT_REPO)/$(_REDHAT_DOCKER_USER)/$(_DOCKER_IMAGE):$(_REDHAT_DOCKER_IMAGE_TAG) -f Dockerfile .

.PHONY: docker-build-all
docker-build-all: docker-build-docker docker-build-redhat  ## Build upstream and Red Hat docker images.

.PHONY: docker-login-docker
docker-login-docker:  ## Login to Dockerhub.
	@docker login

.PHONY: docker-push-gitlab
docker-push-gitlab:  # Tag and push to gitlab.
	docker push $(_GITLAB_REPO)/$(_GITLAB_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG)

.PHONY: docker-push-dockerhub
docker-push-dockerhub: docker-login-docker  # Tag and push to dockerhub.
	docker tag $(_GITLAB_REPO)/$(_GITLAB_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG) $(_DOCKER_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG)
	docker push $(_DOCKER_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG)

.PHONY: docker-push-redhat
docker-push-redhat:  ## Login, tag and push to Red Hat.
	@# Ref: https://connect.redhat.com/projects/5e9f4fa0ebed1415210b4b24/images/upload-image
	@echo "The password is a token acquired by https://connect.redhat.com/projects/5e9f4fa0ebed1415210b4b24/images/upload-image"
	docker login -u unused scan.connect.redhat.com
	docker tag $(_GITLAB_REPO)/$(_GITLAB_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG) scan.connect.redhat.com/ospid-956ccd64-1dcf-4d00-ba98-336497448906/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG)
	docker push scan.connect.redhat.com/ospid-956ccd64-1dcf-4d00-ba98-336497448906/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG)

.PHONY: docker-push-gitlab-registry
docker-push-gitlab-registry: docker-build-docker  ## Build, tag and push to gitlab (recommended for dev).
	@echo -e $(_begin)
	$(eval _TARGET_IMAGE=$(_GITLAB_REPO)/$(_GITLAB_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG))
	docker login $(_GITLAB_REPO)
	docker tag $(_GITLAB_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG) $(_TARGET_IMAGE)
	docker push $(_TARGET_IMAGE)
	@echo -e $(_finish)

.PHONY: docker-push-all
docker-push-all: docker-push-gitlab docker-push-redhat docker-push-dockerhub  ## Push to both Gitlab, Red Hat and Dockerhub.

.PHONY: buildlocal
buildlocal: build docker-build clean

.PHONY: all
all: build docker-build docker-push clean

.PHONY: docker-image-save
docker-image-save: ## Save image to gzipped tar file to _art_dir.
	mkdir -p $(_art_dir) && \
	docker save $(_DOCKER_USER)/$(_DOCKER_IMAGE):$(_DOCKER_IMAGE_TAG) \
		| gzip > ./$(_art_dir)/$(_DOCKER_IMAGE)_$(_DOCKER_IMAGE_TAG)_docker-image.tar.gz
	docker save $(_REDHAT_REPO)/$(_REDHAT_DOCKER_USER)/$(_DOCKER_IMAGE):$(_REDHAT_DOCKER_IMAGE_TAG) \
		| gzip > ./$(_art_dir)/ubi_$(_DOCKER_IMAGE)_$(_REDHAT_DOCKER_IMAGE_TAG)_docker-image.tar.gz

.PHONY: docker-helm-chart-save
docker-helm-chart-save:  ## Save the helm chart to a tarball in _art_dir.
	mkdir -p $(_art_dir) && \
	tar cvfz ./$(_art_dir)/$(_DOCKER_IMAGE)_$(_DOCKER_IMAGE_TAG)_helm-chart.tar.gz deploy/helm
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


