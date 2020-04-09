# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

REDHAT_REPO=scan.connect.redhat.com
BINARY_NAME=infinibox-csi-driver
DOCKER_IMAGE=infinidat-csi-driver

# For Development Build #################################################################
# Docker.io username and tag
DOCKER_USER=nikhilbarge
DOCKER_IMAGE_TAG=test1

# redhat username and tag
REDHAT_DOCKER_USER=nikhilbarge
REDHAT_DOCKER_IMAGE_TAG=ubi8test1
# For Development Build #################################################################


# For Production Build ##################################################################
ifeq ($(env),prod)
	# For Production
	# Do not change following values unless change in production version or username
	#For docker.io  
	DOCKER_USER=infinidat
	DOCKER_IMAGE_TAG=1.1.0.6

	# For scan.connect.redhat.com
	REDHAT_DOCKER_USER=ospid-b731bd28-18bf-4a91-ad3f-8a90853cf5ab
	REDHAT_DOCKER_IMAGE_TAG=1.1.0.1
endif
# For Production Build ##################################################################


clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

build:
	$(GOBUILD) -o $(BINARY_NAME) -v

test: 
	$(GOTEST) -v ./...
  
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

modverify:
	$(GOMOD) verify

modtidy:
	$(GOMOD) tidy

moddownload:
	$(GOMOD) download

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME) -v

docker-build:
	docker build -t $(DOCKER_USER)/$(DOCKER_IMAGE):$(DOCKER_IMAGE_TAG) -f Dockerfile .
	docker build -t $(REDHAT_REPO)/$(REDHAT_DOCKER_USER)/$(DOCKER_IMAGE):$(REDHAT_DOCKER_IMAGE_TAG) -f Dockerfile-ubi .

docker-push:
	docker push $(DOCKER_USER)/$(DOCKER_IMAGE):$(DOCKER_IMAGE_TAG)
	docker push $(REDHAT_REPO)/$(REDHAT_DOCKER_USER)/$(DOCKER_IMAGE):$(REDHAT_DOCKER_IMAGE_TAG)

buildlocal: build docker-build clean

all: build docker-build docker-push clean
