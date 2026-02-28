#!/bin/bash
#
# Build a .deb package for athena-dhcpd.
#
# Usage: ./scripts/build-deb.sh [version] [arch]
#   version  — package version (default: git describe or "0.0.0-dev")
#   arch     — target architecture: amd64, arm64 (default: amd64)
#
# Expects the Go binary to already be built at build/athena-dhcpd
# (run `make build` first, or the CI workflow handles this)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION="${1:-$(git -C "$PROJECT_DIR" describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "0.0.0-dev")}"
ARCH="${2:-amd64}"
BINARY="${PROJECT_DIR}/build/athena-dhcpd"

if [ ! -f "$BINARY" ]; then
    echo "ERROR: Binary not found at $BINARY" >&2
    echo "Run 'make build' first or set GOOS=linux GOARCH=<arch> when building." >&2
    exit 1
fi

PKG_NAME="athena-dhcpd_${VERSION}_${ARCH}"
STAGING="${PROJECT_DIR}/build/${PKG_NAME}"

echo "==> Building .deb package: ${PKG_NAME}.deb"
echo "    Version: ${VERSION}"
echo "    Arch:    ${ARCH}"

# Clean staging area
rm -rf "$STAGING"

# Create directory structure
mkdir -p "$STAGING/DEBIAN"
mkdir -p "$STAGING/usr/bin"
mkdir -p "$STAGING/etc/athena-dhcpd"
mkdir -p "$STAGING/lib/systemd/system"
mkdir -p "$STAGING/usr/share/doc/athena-dhcpd"
mkdir -p "$STAGING/var/lib/athena-dhcpd"

# Install binaries
install -m 0755 "$BINARY" "$STAGING/usr/bin/athena-dhcpd"
if [ -f "$PROJECT_DIR/build/athena-hashpw" ]; then
    install -m 0755 "$PROJECT_DIR/build/athena-hashpw" "$STAGING/usr/bin/athena-hashpw"
fi

# Install config (example as default)
install -m 0640 "$PROJECT_DIR/configs/example.toml" "$STAGING/etc/athena-dhcpd/config.toml"

# Install systemd service (fix path: /usr/local/bin → /usr/bin for deb)
sed 's|/usr/local/bin/athena-dhcpd|/usr/bin/athena-dhcpd|g' \
    "$PROJECT_DIR/deploy/athena-dhcpd.service" > "$STAGING/lib/systemd/system/athena-dhcpd.service"
chmod 0644 "$STAGING/lib/systemd/system/athena-dhcpd.service"

# Install docs
if [ -d "$PROJECT_DIR/docs" ]; then
    cp -r "$PROJECT_DIR/docs"/* "$STAGING/usr/share/doc/athena-dhcpd/"
fi
install -m 0644 "$PROJECT_DIR/debian/copyright" "$STAGING/usr/share/doc/athena-dhcpd/copyright"

# Build DEBIAN/control with version and arch substituted
sed -e "s/{{VERSION}}/${VERSION}/g" -e "s/{{ARCH}}/${ARCH}/g" \
    "$PROJECT_DIR/debian/control" > "$STAGING/DEBIAN/control"

# Calculate installed size (in KB)
INSTALLED_SIZE=$(du -sk "$STAGING" | cut -f1)
echo "Installed-Size: ${INSTALLED_SIZE}" >> "$STAGING/DEBIAN/control"

# Install maintainer scripts
for script in postinst prerm postrm; do
    if [ -f "$PROJECT_DIR/debian/${script}" ]; then
        install -m 0755 "$PROJECT_DIR/debian/${script}" "$STAGING/DEBIAN/${script}"
    fi
done

# Install conffiles
install -m 0644 "$PROJECT_DIR/debian/conffiles" "$STAGING/DEBIAN/conffiles"

# Build the .deb
dpkg-deb --build --root-owner-group "$STAGING" "${PROJECT_DIR}/build/${PKG_NAME}.deb"

echo "==> Package built: build/${PKG_NAME}.deb"

# Show package info
dpkg-deb --info "${PROJECT_DIR}/build/${PKG_NAME}.deb"

# Cleanup staging
rm -rf "$STAGING"
