# Sendry Makefile

# Ensure Go bin is in PATH for tools like nfpm
export PATH := $(PATH):$(HOME)/go/bin

# Variables
BINARY_NAME=sendry
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Directories
BUILD_DIR=build
CMD_DIR=./cmd/sendry

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for all platforms
.PHONY: build-all
build-all: build-linux build-linux-arm64 build-darwin build-darwin-arm64

.PHONY: build-linux
build-linux:
	@echo "Building for Linux amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

.PHONY: build-linux-arm64
build-linux-arm64:
	@echo "Building for Linux arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)

.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)

.PHONY: build-darwin-arm64
build-darwin-arm64:
	@echo "Building for macOS arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	@mkdir -p $(BUILD_DIR)
	$(GOTEST) -v -coverprofile=$(BUILD_DIR)/coverage.out ./...
	$(GOCMD) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"

# Run tests with race detector
.PHONY: test-race
test-race:
	@echo "Running tests with race detector..."
	$(GOTEST) -v -race ./...

# Run short tests only
.PHONY: test-short
test-short:
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

# Benchmark tests
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Vet the code
.PHONY: vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Format the code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Check formatting
.PHONY: fmt-check
fmt-check:
	@echo "Checking code formatting..."
	@diff=$$($(GOFMT) -s -d .); \
	if [ -n "$$diff" ]; then \
		echo "Code is not formatted:"; \
		echo "$$diff"; \
		exit 1; \
	fi
	@echo "Code formatting OK"

# Lint (requires golangci-lint)
.PHONY: lint
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

# Tidy dependencies
.PHONY: tidy
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

# Verify dependencies
.PHONY: verify
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

# Install binary to GOPATH/bin
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# Run the application (for development)
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME) serve -c configs/sendry.example.yaml

# Generate DKIM key for testing
.PHONY: dkim-gen
dkim-gen: build
	@echo "Generating DKIM key..."
	@mkdir -p $(BUILD_DIR)/keys
	$(BUILD_DIR)/$(BINARY_NAME) dkim generate --domain example.com --selector sendry --out $(BUILD_DIR)/keys

# Run all checks (for CI)
.PHONY: ci
ci: deps fmt-check vet test

# Full check (format, vet, lint, test)
.PHONY: check
check: fmt vet test

# Show version info
.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

# Docker image name
DOCKER_IMAGE=sendry
DOCKER_TAG?=$(VERSION)

# Docker build
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Docker build with no cache
.PHONY: docker-build-nc
docker-build-nc:
	@echo "Building Docker image (no cache)..."
	docker build --no-cache \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.

# Docker build from APK package (smaller image)
.PHONY: docker-build-apk
docker-build-apk: package-apk
	@echo "Building Docker image from APK package..."
	docker build \
		-f Dockerfile.apk \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Docker run
.PHONY: docker-run
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it \
		-p 25:25 -p 587:587 -p 8080:8080 \
		-v $(PWD)/configs/sendry.example.yaml:/etc/sendry/config.yaml:ro \
		$(DOCKER_IMAGE):latest

# Docker compose up
.PHONY: docker-up
docker-up:
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) \
	docker compose up -d --build

# Docker compose down
.PHONY: docker-down
docker-down:
	docker compose down

# Docker compose logs
.PHONY: docker-logs
docker-logs:
	docker compose logs -f

# Build DEB package (requires nfpm)
.PHONY: package-deb
package-deb: build-linux
	@echo "Building DEB package..."
	@which nfpm > /dev/null || (echo "nfpm not installed. Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" && exit 1)
	@mkdir -p $(BUILD_DIR)/packages
	@sed 's/$${ARCH}/amd64/g; s/$${VERSION}/$(VERSION)/g' nfpm.yaml > $(BUILD_DIR)/nfpm-amd64.yaml
	nfpm package --config $(BUILD_DIR)/nfpm-amd64.yaml --packager deb --target $(BUILD_DIR)/packages/
	@echo "DEB package built in $(BUILD_DIR)/packages/"

# Build DEB package for arm64
.PHONY: package-deb-arm64
package-deb-arm64: build-linux-arm64
	@echo "Building DEB package for arm64..."
	@which nfpm > /dev/null || (echo "nfpm not installed" && exit 1)
	@mkdir -p $(BUILD_DIR)/packages
	@sed 's/$${ARCH}/arm64/g; s/$${VERSION}/$(VERSION)/g' nfpm.yaml > $(BUILD_DIR)/nfpm-arm64.yaml
	nfpm package --config $(BUILD_DIR)/nfpm-arm64.yaml --packager deb --target $(BUILD_DIR)/packages/
	@echo "DEB package built in $(BUILD_DIR)/packages/"

# Build RPM package (requires nfpm)
.PHONY: package-rpm
package-rpm: build-linux
	@echo "Building RPM package..."
	@which nfpm > /dev/null || (echo "nfpm not installed. Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" && exit 1)
	@mkdir -p $(BUILD_DIR)/packages
	@sed 's/$${ARCH}/amd64/g; s/$${VERSION}/$(VERSION)/g' nfpm.yaml > $(BUILD_DIR)/nfpm-amd64.yaml
	nfpm package --config $(BUILD_DIR)/nfpm-amd64.yaml --packager rpm --target $(BUILD_DIR)/packages/
	@echo "RPM package built in $(BUILD_DIR)/packages/"

# Build RPM package for arm64
.PHONY: package-rpm-arm64
package-rpm-arm64: build-linux-arm64
	@echo "Building RPM package for arm64..."
	@which nfpm > /dev/null || (echo "nfpm not installed" && exit 1)
	@mkdir -p $(BUILD_DIR)/packages
	@sed 's/$${ARCH}/arm64/g; s/$${VERSION}/$(VERSION)/g' nfpm.yaml > $(BUILD_DIR)/nfpm-arm64.yaml
	nfpm package --config $(BUILD_DIR)/nfpm-arm64.yaml --packager rpm --target $(BUILD_DIR)/packages/
	@echo "RPM package built in $(BUILD_DIR)/packages/"

# Build APK package for Alpine (amd64)
.PHONY: package-apk
package-apk: build-linux
	@echo "Building APK package..."
	@which nfpm > /dev/null || (echo "nfpm not installed. Install: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" && exit 1)
	@mkdir -p $(BUILD_DIR)/packages
	@sed 's/$${ARCH}/amd64/g; s/$${VERSION}/$(VERSION)/g' nfpm.yaml > $(BUILD_DIR)/nfpm-amd64.yaml
	nfpm package --config $(BUILD_DIR)/nfpm-amd64.yaml --packager apk --target $(BUILD_DIR)/packages/
	@echo "APK package built in $(BUILD_DIR)/packages/"

# Build APK package for Alpine (arm64)
.PHONY: package-apk-arm64
package-apk-arm64: build-linux-arm64
	@echo "Building APK package for arm64..."
	@which nfpm > /dev/null || (echo "nfpm not installed" && exit 1)
	@mkdir -p $(BUILD_DIR)/packages
	@sed 's/$${ARCH}/arm64/g; s/$${VERSION}/$(VERSION)/g' nfpm.yaml > $(BUILD_DIR)/nfpm-arm64.yaml
	nfpm package --config $(BUILD_DIR)/nfpm-arm64.yaml --packager apk --target $(BUILD_DIR)/packages/
	@echo "APK package built in $(BUILD_DIR)/packages/"

# Build all packages
.PHONY: package-all
package-all: package-deb package-deb-arm64 package-rpm package-rpm-arm64 package-apk package-apk-arm64
	@echo "All packages built in $(BUILD_DIR)/packages/"

# Full release (binaries + packages + docker)
.PHONY: release
release: clean build-all package-all docker-build
	@echo ""
	@echo "Release $(VERSION) complete!"
	@echo "Binaries: $(BUILD_DIR)/"
	@echo "Packages: $(BUILD_DIR)/packages/"
	@echo "Docker:   $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Help
.PHONY: help
help:
	@echo "Sendry Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@echo "  build            Build the binary"
	@echo "  build-all        Build for all platforms (linux, darwin, amd64, arm64)"
	@echo "  clean            Clean build artifacts"
	@echo "  install          Install binary to GOPATH/bin"
	@echo ""
	@echo "Test targets:"
	@echo "  test             Run tests"
	@echo "  test-cover       Run tests with coverage report"
	@echo "  test-race        Run tests with race detector"
	@echo "  bench            Run benchmarks"
	@echo ""
	@echo "Code quality:"
	@echo "  fmt              Format code"
	@echo "  fmt-check        Check code formatting"
	@echo "  vet              Run go vet"
	@echo "  lint             Run linter (requires golangci-lint)"
	@echo "  check            Run all checks (fmt, vet, test)"
	@echo "  ci               Run CI checks"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build     Build Docker image (multi-stage)"
	@echo "  docker-build-apk Build Docker image from APK package"
	@echo "  docker-build-nc  Build Docker image (no cache)"
	@echo "  docker-run       Run Docker container"
	@echo "  docker-up        Docker compose up"
	@echo "  docker-down      Docker compose down"
	@echo "  docker-logs      Docker compose logs"
	@echo ""
	@echo "Package targets:"
	@echo "  package-deb      Build DEB package (amd64)"
	@echo "  package-deb-arm64 Build DEB package (arm64)"
	@echo "  package-rpm      Build RPM package (amd64)"
	@echo "  package-rpm-arm64 Build RPM package (arm64)"
	@echo "  package-apk      Build APK package for Alpine (amd64)"
	@echo "  package-apk-arm64 Build APK package for Alpine (arm64)"
	@echo "  package-all      Build all packages"
	@echo ""
	@echo "Release:"
	@echo "  release          Full release (binaries + packages + docker)"
	@echo ""
	@echo "Other:"
	@echo "  run              Build and run the application"
	@echo "  dkim-gen         Generate test DKIM key"
	@echo "  deps             Download dependencies"
	@echo "  tidy             Tidy dependencies"
	@echo "  version          Show version info"
	@echo "  help             Show this help"
