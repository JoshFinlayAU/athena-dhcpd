.PHONY: build build-web dev test lint clean install run

BINARY_NAME := athena-dhcpd
BUILD_DIR := build
WEB_DIR := web
WEB_DIST := $(WEB_DIR)/dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

# Build the Go binary (depends on frontend if web dir exists)
build: $(if $(wildcard $(WEB_DIR)/package.json),build-web)
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/athena-dhcpd

# Build the frontend (React + Vite)
build-web:
	@if [ -d "$(WEB_DIR)" ] && [ -f "$(WEB_DIR)/package.json" ]; then \
		cd $(WEB_DIR) && npm ci && npm run build; \
	fi

# Run in development mode
dev:
	go run ./cmd/athena-dhcpd -config configs/example.toml

# Run tests
test:
	go test -v -race -count=1 ./...

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	@if [ -d "$(WEB_DIST)" ]; then rm -rf $(WEB_DIST); fi

# Install binary to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/athena-dhcpd

# Run the server (requires root/CAP_NET_RAW for ARP probing)
run: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -config configs/example.toml

# Format code
fmt:
	gofmt -w .

# Check that code compiles
check:
	go build ./...
