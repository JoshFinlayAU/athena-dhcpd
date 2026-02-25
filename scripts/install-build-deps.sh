#!/bin/bash
#
# Install build dependencies for athena-dhcpd on Debian 12 (bookworm) / Debian 13 (trixie)
#
# Usage: sudo ./scripts/install-build-deps.sh
#
set -euo pipefail

# must be root
if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: run this as root (sudo $0)" >&2
    exit 1
fi

# figure out what we're running on
if [ ! -f /etc/os-release ]; then
    echo "ERROR: cant find /etc/os-release, is this even debian?" >&2
    exit 1
fi

. /etc/os-release

if [ "${ID:-}" != "debian" ]; then
    echo "WARNING: this script is for debian, you're running ${ID:-unknown}. continuing anyway..." >&2
fi

VERSION_ID="${VERSION_ID:-0}"
echo "==> Detected: ${PRETTY_NAME:-Debian ${VERSION_ID}}"

if [ "$VERSION_ID" != "12" ] && [ "$VERSION_ID" != "13" ]; then
    echo "WARNING: this script targets debian 12/13, you're on ${VERSION_ID}. might still work" >&2
fi

# base build tools
echo "==> Installing base build tools..."
apt-get update
apt-get install -y --no-install-recommends \
    build-essential \
    git \
    curl \
    ca-certificates \
    gnupg \
    dpkg-dev \
    apt-utils

# golang — debian 12 ships 1.19 which is too old, debian 13 ships 1.23+
# we install from the official go repo to get a recent enough version
GO_VERSION="1.23.6"
GO_INSTALLED=""

if command -v go >/dev/null 2>&1; then
    GO_INSTALLED=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' || true)
    echo "==> Found existing Go ${GO_INSTALLED}"
fi

# need at least 1.22
install_go() {
    echo "==> Installing Go ${GO_VERSION} from golang.org..."
    ARCH=$(dpkg --print-architecture)
    case "$ARCH" in
        amd64) GOARCH="amd64" ;;
        arm64) GOARCH="arm64" ;;
        armhf) GOARCH="armv6l" ;;
        *)
            echo "ERROR: unsupported architecture: ${ARCH}" >&2
            exit 1
            ;;
    esac

    TARBALL="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
    curl -fsSL "https://go.dev/dl/${TARBALL}" -o "/tmp/${TARBALL}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${TARBALL}"
    rm -f "/tmp/${TARBALL}"

    # make sure its on PATH for this script and future shells
    if ! grep -q '/usr/local/go/bin' /etc/profile.d/golang.sh 2>/dev/null; then
        echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/golang.sh
        chmod 0644 /etc/profile.d/golang.sh
    fi
    export PATH=$PATH:/usr/local/go/bin

    echo "==> Installed $(go version)"
}

if [ -z "$GO_INSTALLED" ]; then
    install_go
else
    GO_MAJOR=$(echo "$GO_INSTALLED" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_INSTALLED" | cut -d. -f2)
    if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 22 ]); then
        echo "==> Go ${GO_INSTALLED} is too old (need >= 1.22), upgrading..."
        install_go
    else
        echo "==> Go ${GO_INSTALLED} is good enough, skipping install"
    fi
fi

# node.js — need a reasonably modern version for vite/react build
# debian 12 ships node 18, debian 13 ships node 20. both are fine
echo "==> Installing Node.js and npm..."
if [ "$VERSION_ID" = "12" ]; then
    # bookworm has node 18 in repos, thats fine for vite
    apt-get install -y --no-install-recommends nodejs npm
else
    # trixie (13) has node 20+
    apt-get install -y --no-install-recommends nodejs npm
fi

echo "==> Node $(node --version), npm $(npm --version)"

# staticcheck (optional, for `make lint`)
echo "==> Installing staticcheck (optional)..."
if command -v go >/dev/null 2>&1; then
    GOBIN=/usr/local/bin go install honnef.co/go/tools/cmd/staticcheck@latest 2>/dev/null || \
        echo "    staticcheck install failed, lint will skip it (non-fatal)"
fi

# summary
echo ""
echo "========================================="
echo "  Build dependencies installed"
echo "========================================="
echo "  Go:     $(go version 2>/dev/null || echo 'not found')"
echo "  Node:   $(node --version 2>/dev/null || echo 'not found')"
echo "  npm:    $(npm --version 2>/dev/null || echo 'not found')"
echo "  git:    $(git --version 2>/dev/null || echo 'not found')"
echo "  dpkg:   $(dpkg-deb --version 2>/dev/null | head -1 || echo 'not found')"
echo "========================================="
echo ""
echo "you can now run:"
echo "  make build       # build everything"
echo "  make build-deb   # build .deb package"
echo "  make test        # run tests"
echo ""
echo "if you just installed go, you may need to:"
echo "  source /etc/profile.d/golang.sh"
echo "or just open a new shell"
