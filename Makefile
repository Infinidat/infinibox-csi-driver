# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_NAME=infinibox-csi-driver
DOCKER_USER=infinidat
DOCKER_IMAGE=infinidat-csi-driver
DOCKER_IMAGE_TAG= 1.1.0.3

all: build docker-build docker-push

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
	docker build -t $(DOCKER_USER)/$(DOCKER_IMAGE):$(DOCKER_IMAGE_TAG) .
	
docker-push:
	docker push $(DOCKER_USER)/$(DOCKER_IMAGE):$(DOCKER_IMAGE_TAG)

buildlocal: build docker-build


