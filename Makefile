.PHONY: help build test clean install deploy docker-build docker-push

# Variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
VERSION ?= $(shell git describe --tags --always --dirty)
REGISTRY ?= ghcr.io/ewerton-silva00
PROJECT_PATH ?= cloudstack/karpenter

# Container Image
IMAGE_NAME ?= $(PROJECT_PATH)/controller
IMAGE_TAG ?= $(VERSION)
IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Tools
GOLANGCI_LINT_VERSION ?= v2.15.0

# Helm Chart
CHART_NAME ?= $(PROJECT_PATH)/karpenter
CHART_VERSION ?= $(VERSION)

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

tidy: ## Run go mod tidy
	go mod tidy

fmt: ## Run go fmt
	go fmt ./...

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	$(shell go env GOPATH)/bin/golangci-lint run

test: ## Run unit tests
	go test -v ./...

test-integration: ## Run integration tests (requires CloudStack environment)
	go test -v -tags=integration ./test/integration/...

coverage: test ## Generate coverage report
	go tool cover -html=coverage.out -o coverage.html

##@ Build

build: ## Build the controller binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-X main.version=$(VERSION)" -o bin/controller cmd/controller/main.go

install: build ## Install the controller binary
	cp bin/controller $(GOPATH)/bin/karpenter-cloudstack-controller

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html

##@ Docker

docker-build: ## Build docker image for current platform
	docker build -t $(IMAGE) \
		--build-arg VERSION=$(VERSION) \
		.
	docker tag $(IMAGE) $(REGISTRY)/$(IMAGE_NAME):latest

docker-build-multiarch: ## Build docker image for multiple architectures (amd64, arm64)
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--load \
		.

docker-push: ## Push docker image
	docker push $(IMAGE)
	docker push $(REGISTRY)/$(IMAGE_NAME):latest

docker-push-multiarch: ## Build and push multi-arch docker images
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--push \
		.

##@ Deployment

generate: ## Generate CRDs and other manifests
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/..."
	controller-gen crd:generateEmbeddedObjectMeta=true paths="./pkg/apis/..." output:crd:artifacts:config=charts/karpenter-cloudstack/crds

deploy: ## Deploy to Kubernetes cluster
	helm upgrade --install karpenter-cloudstack ./charts/karpenter-cloudstack \
		--namespace karpenter --create-namespace \
		--set image.tag=$(IMAGE_TAG)

undeploy: ## Remove from Kubernetes cluster
	helm uninstall karpenter-cloudstack --namespace karpenter

##@ Tools

tools: ## Install development tools
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

verify: tidy fmt vet lint test ## Run all verification checks

