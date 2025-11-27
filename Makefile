# Variables
APP_NAME=vm-deep-inspection-demo
VERSION?=latest
REGISTRY?=localhost:5000
IMAGE_NAME=$(REGISTRY)/$(APP_NAME):$(VERSION)
LOCAL_IMAGE_NAME=$(APP_NAME):$(VERSION)

# Container runtime (docker or podman)
CONTAINER_RUNTIME?=podman

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod
BINARY_NAME=vm-inspector
BINARY_DIR=./bin
BINARY_PATH=$(BINARY_DIR)/$(BINARY_NAME)
MAIN_PATH=./cmd/server
# Build flags
LDFLAGS=-ldflags "-s -w"
BUILD_FLAGS=-trimpath

.PHONY: all build clean deps docker-build docker-run docker-stop docker-logs docker-shell docker-test-virt docker-test-vddk swagger help run run-config

all: deps build

# =============================================================================
# Go Build Targets
# =============================================================================

## Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "Build complete: $(BINARY_PATH)"

## Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BINARY_DIR)
	@rm -f vm-inspector
	@echo "Clean complete"

## Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# =============================================================================
# Container Targets (Basic)
# =============================================================================

docker-build:
	@echo "Building container image (VDDK is mounted at runtime) using $(CONTAINER_RUNTIME)..."
	$(CONTAINER_RUNTIME) build --platform linux/amd64 -f Dockerfile.vddk -t $(LOCAL_IMAGE_NAME) .
	@echo "Container image built: $(LOCAL_IMAGE_NAME)"

docker-run:
	@echo "Starting container with VDDK mounted from host using $(CONTAINER_RUNTIME)..."
	@$(CONTAINER_RUNTIME) rm -f vm-inspector 2>/dev/null || true

	$(CONTAINER_RUNTIME) run -d \
		-p 8080:8080 \
		-v $(PWD)/config.yaml:/etc/vm-inspector/config.yaml:ro \
		-v /opt/vmware-vix-disklib:/opt/vmware-vix-disklib:ro \
		-e LIBGUESTFS_BACKEND=direct \
		-e LD_LIBRARY_PATH=/opt/vmware-vix-disklib/lib64 \
		--privileged \
		--device /dev/kvm:/dev/kvm \
		--name vm-inspector \
		$(LOCAL_IMAGE_NAME)

	@echo "Container started: vm-inspector (VDDK mounted from host)"
	@echo "API available at: http://localhost:8080"
	@echo ""
	@echo "Useful commands:"
	@echo "  make docker-logs       - View container logs"
	@echo "  make docker-shell      - Open shell in container"
	@echo "  make docker-test-vddk  - Test VDDK/nbdkit"
	@echo "  make docker-stop       - Stop container"

## Stop container
docker-stop:
	@echo "Stopping container..."
	$(CONTAINER_RUNTIME) stop vm-inspector
	$(CONTAINER_RUNTIME) rm vm-inspector
	@echo "Container stopped and removed"

## View container logs
docker-logs:
	$(CONTAINER_RUNTIME) logs -f vm-inspector

## Open shell in running container
docker-shell:
	@echo "Opening shell in container..."
	@echo "Test virt-inspector with: virt-inspector --version"
	$(CONTAINER_RUNTIME) exec -it vm-inspector bash

## Test VDDK and nbdkit availability in container
docker-test-vddk:
	@echo "Testing VDDK and nbdkit in container..."
	@echo ""
	@echo "=== virt-inspector version ==="
	@$(CONTAINER_RUNTIME) exec vm-inspector virt-inspector --version || echo "virt-inspector not found"
	@echo ""
	@echo "=== nbdkit version ==="
	@$(CONTAINER_RUNTIME) exec vm-inspector nbdkit --version || echo "nbdkit not found"
	@echo ""
	@echo "=== nbdkit-vddk-plugin ==="
	@$(CONTAINER_RUNTIME) exec vm-inspector nbdkit vddk --version || echo "nbdkit-vddk-plugin not found"
	@echo ""
	@echo "=== VDDK libraries ==="
	@$(CONTAINER_RUNTIME) exec --user root vm-inspector sh -c 'ls -lh /opt/vmware-vix-disklib/lib64/libvixDiskLib.so*' || echo "VDDK libraries not found"

# =============================================================================
# Development Targets
# =============================================================================

## Run the application locally
run: build
	$(BINARY_PATH)

## Run with config file
run-config: build
	$(BINARY_PATH) -config config.yaml

## Generate OpenAPI documentation
swagger:
	swag init -g cmd/server/main.go -o docs

## Install swag tool
install-swag:
	go install github.com/swaggo/swag/cmd/swag@latest

## Show help
help:
	@echo ''
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)
