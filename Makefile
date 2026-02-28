.PHONY: build build-web build-deb apt-repo dev test lint clean install run hashpw

BINARY_NAME := athena-dhcpd
BUILD_DIR := build
WEB_DIR := web
WEB_DIST := $(WEB_DIR)/dist
EMBED_DIR := internal/webui/dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

# Build the Go binary (depends on frontend if web dir exists)
build: $(if $(wildcard $(WEB_DIR)/package.json),build-web)
	@mkdir -p $(BUILD_DIR)
	@rm -rf $(EMBED_DIR) && mkdir -p $(EMBED_DIR)
	@if [ -d "$(WEB_DIST)" ]; then cp -r $(WEB_DIST)/* $(EMBED_DIR)/; fi
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/athena-dhcpd
	go build $(LDFLAGS) -o $(BUILD_DIR)/athena-hashpw ./cmd/athena-hashpw

# Build the frontend (React + Vite)
build-web:
	@if [ -d "$(WEB_DIR)" ] && [ -f "$(WEB_DIR)/package.json" ]; then \
		cd $(WEB_DIR) && npm ci && npm run build; \
	fi

# Build .deb package (requires dpkg-deb, linux only)
build-deb: build
	@chmod +x scripts/build-deb.sh
	scripts/build-deb.sh $(VERSION:v%=%)

# Build APT repo structure from .deb packages in build/
apt-repo: build-deb
	@mkdir -p apt/pool/main apt/dists/stable/main/binary-amd64 apt/dists/stable/main/binary-arm64
	@cp -f $(BUILD_DIR)/*.deb apt/pool/main/
	@cd apt && dpkg-scanpackages pool/main /dev/null > dists/stable/main/binary-amd64/Packages
	@cd apt && gzip -k -f dists/stable/main/binary-amd64/Packages
	@cd apt && apt-ftparchive release dists/stable > dists/stable/Release
	@echo "APT repo built in apt/"

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
	@rm -rf $(EMBED_DIR)
	@rm -f apt/pool/main/*.deb apt/dists/stable/Release* apt/dists/stable/InRelease
	@rm -f apt/dists/stable/main/binary-*/Packages*

# Install everything to the system (run as root or with sudo)
install: build
	@echo "==> Installing athena-dhcpd..."
	install -d /usr/local/bin
	install -m 0755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	install -m 0755 $(BUILD_DIR)/athena-hashpw /usr/local/bin/athena-hashpw
	@# Config directory (775 so service can write backups + temp files)
	install -d -m 0775 /etc/athena-dhcpd
	@if [ ! -f /etc/athena-dhcpd/config.toml ]; then \
		install -m 0660 configs/example.toml /etc/athena-dhcpd/config.toml; \
		echo "    Installed example config to /etc/athena-dhcpd/config.toml"; \
	else \
		echo "    Config already exists, not overwriting"; \
	fi
	@# Data directory
	install -d -m 0750 /var/lib/athena-dhcpd
	@# Systemd service
	@if [ -d /etc/systemd/system ]; then \
		install -m 0644 deploy/athena-dhcpd.service /etc/systemd/system/athena-dhcpd.service; \
		systemctl daemon-reload; \
		echo "    Installed systemd service"; \
	fi
	@# Capabilities for raw sockets, port binding, and VIP management
	@if command -v setcap >/dev/null 2>&1; then \
		setcap 'cap_net_raw,cap_net_bind_service,cap_net_admin+ep' /usr/local/bin/$(BINARY_NAME); \
		echo "    Set CAP_NET_RAW, CAP_NET_BIND_SERVICE, CAP_NET_ADMIN on binary"; \
	else \
		echo "    WARNING: setcap not found, install libcap2-bin or run as root"; \
	fi
	@echo "==> Done. Edit /etc/athena-dhcpd/config.toml then run:"
	@echo "    systemctl enable --now athena-dhcpd"

# Run the server (requires root/CAP_NET_RAW for ARP probing)
run: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) -config configs/example.toml

# Build just the password hash tool
hashpw:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/athena-hashpw ./cmd/athena-hashpw

# Format code
fmt:
	gofmt -w .

# Check that code compiles
check:
	go build ./...
