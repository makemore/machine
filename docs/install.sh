#!/bin/sh
set -e

# mach installer — detects OS/arch and downloads the right binary
# Usage: curl -fsSL https://makemore.github.io/machine/install.sh | sh

REPO="makemore/machine"
VERSION="v0.1.0"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  darwin) OS="darwin" ;;
  linux)  OS="linux" ;;
  *)
    echo "Error: unsupported OS: $OS"
    echo "mach supports macOS (darwin) and Linux"
    exit 1
    ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)   ARCH="amd64" ;;
  arm64|aarch64)   ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH"
    echo "mach supports amd64 and arm64"
    exit 1
    ;;
esac

BINARY="mach-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}"

echo ""
echo "  ⚡ Installing mach ${VERSION}"
echo "     OS:   ${OS}"
echo "     Arch: ${ARCH}"
echo ""

# Download
TMP=$(mktemp -d)
echo "  📥 Downloading ${URL}..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "${TMP}/mach" "${URL}"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "${TMP}/mach" "${URL}"
else
  echo "Error: curl or wget required"
  exit 1
fi

chmod +x "${TMP}/mach"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/mach" "${INSTALL_DIR}/mach"
else
  echo "  🔐 Need sudo to install to ${INSTALL_DIR}"
  sudo mv "${TMP}/mach" "${INSTALL_DIR}/mach"
fi

rm -rf "${TMP}"

echo "  ✅ Installed mach to ${INSTALL_DIR}/mach"
echo ""
echo "  Get started:"
echo "    mach doctor    # check dependencies"
echo "    mach up        # spin up a VM"
echo ""

