#!/bin/sh
# rest-mcp installer — downloads the latest release for the current platform.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/devstroop/rest-mcp/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/devstroop/rest-mcp/main/install.sh | sh -s -- v0.2.0
#
# Environment variables:
#   INSTALL_DIR  — installation directory (default: /usr/local/bin or ~/.local/bin)

set -e

REPO="devstroop/rest-mcp"
BINARY="rest-mcp"

# Determine version
VERSION="${1:-latest}"
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
  if [ -z "$VERSION" ]; then
    echo "Error: could not determine latest version." >&2
    exit 1
  fi
fi

# Strip leading 'v' for the archive filename
VERSION_NUM="${VERSION#v}"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  GOOS="linux" ;;
  darwin) GOOS="darwin" ;;
  *)
    echo "Error: unsupported OS: $OS" >&2
    echo "Download manually from https://github.com/${REPO}/releases" >&2
    exit 1
    ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Install directory
if [ -n "$INSTALL_DIR" ]; then
  BIN_DIR="$INSTALL_DIR"
elif [ -w /usr/local/bin ]; then
  BIN_DIR="/usr/local/bin"
else
  BIN_DIR="$HOME/.local/bin"
  mkdir -p "$BIN_DIR"
fi

ARCHIVE="${BINARY}_${VERSION_NUM}_${GOOS}_${GOARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

echo "Downloading ${BINARY} ${VERSION} for ${GOOS}/${GOARCH}..."
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

curl -fsSL "$URL" -o "${TMPDIR}/${ARCHIVE}"
tar xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

# Install binary
install -m 755 "${TMPDIR}/${BINARY}" "${BIN_DIR}/${BINARY}"

echo ""
echo "${BINARY} ${VERSION} installed to ${BIN_DIR}/${BINARY}"

# Check if BIN_DIR is in PATH
case ":$PATH:" in
  *":${BIN_DIR}:"*) ;;
  *)
    echo ""
    echo "NOTE: ${BIN_DIR} is not in your PATH."
    echo "  Add it with: export PATH=\"${BIN_DIR}:\$PATH\""
    ;;
esac

echo ""
echo "Get started:"
echo "  ${BINARY} --version"
echo "  ${BINARY} --spec openapi.json --base-url https://api.example.com --dry-run"
