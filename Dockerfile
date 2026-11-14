##
# Build stage
##
FROM --platform=$BUILDPLATFORM golang:1.25.4-alpine AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build for target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -ldflags="-X main.version=${VERSION}" \
    -o controller cmd/controller/main.go

##
# Final stage
##
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static:nonroot

# Links the package to the repository on GitHub Container Registry
LABEL org.opencontainers.image.source=https://github.com/ewerton-silva00/karpenter-provider-cloudstack

WORKDIR /

COPY --from=builder /workspace/controller .

USER 65532:65532

ENTRYPOINT ["/controller"]
