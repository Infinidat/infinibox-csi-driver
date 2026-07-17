#
# BUILDER
#
FROM docker.io/library/golang:1.26.4 AS builder

WORKDIR /buildroot

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the source.
#COPY pkg/ pkg/
COPY cmd/ cmd/
#COPY internal/ internal/

# Disable CGO.
ENV CGO_ENABLED=0

# Build the image.
RUN go build -trimpath -o build/infinidat-cosi-test-client cmd/main.go

#
# FINAL IMAGE
#
FROM gcr.io/distroless/static:latest AS runtime

LABEL org.opencontainers.image.maintainers="Kubernetes Authors"
LABEL org.opencontainers.image.description="Container Object Storage Interface (COSI) Sample Driver"
LABEL org.opencontainers.image.title="COSI test client"
LABEL org.opencontainers.image.source="https://github.com/kubernetes-sigs/cosi-driver-sample"
LABEL org.opencontainers.image.licenses="APACHE-2.0"

COPY --from=builder /buildroot/build/infinidat-cosi-test-client /infinidat-cosi-test-client

ENTRYPOINT [ "/infinidat-cosi-test-client" ]
