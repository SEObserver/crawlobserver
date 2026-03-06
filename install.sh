#!/bin/sh
# CrawlObserver installer — detects OS/arch and downloads the right binary.
# Usage: curl -fsSL https://raw.githubusercontent.com/SEObserver/crawlobserver/main/install.sh | sh

set -e

REPO="SEObserver/crawlobserver"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="crawlobserver"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  GOOS="linux" ;;
  Darwin) GOOS="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) GOOS="windows" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  GOARCH="amd64" ;;
  arm64|aarch64) GOARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Windows suffix
SUFFIX=""
if [ "$GOOS" = "windows" ]; then
  SUFFIX=".exe"
  INSTALL_DIR="."
fi

ASSET="${BINARY_NAME}-${GOOS}-${GOARCH}${SUFFIX}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading CrawlObserver for ${GOOS}/${GOARCH}..."
echo "  ${URL}"

# Download to temp file
TMP="$(mktemp)"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TMP"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMP" "$URL"
else
  echo "Error: curl or wget is required."
  exit 1
fi

# Install
if [ "$GOOS" = "windows" ]; then
  mv "$TMP" "${BINARY_NAME}${SUFFIX}"
  echo ""
  echo "Done! Run with: .\\${BINARY_NAME}${SUFFIX}"
else
  chmod +x "$TMP"
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY_NAME}"
  fi
  echo ""
  echo "Done! CrawlObserver installed to ${INSTALL_DIR}/${BINARY_NAME}"
  echo ""
  echo "  crawlobserver"
  echo ""
  echo "Open http://127.0.0.1:8899 and start crawling."
fi
