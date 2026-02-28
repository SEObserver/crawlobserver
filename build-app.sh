#!/bin/bash
set -e

APP_NAME="CrawlObserver"
BUNDLE="build/${APP_NAME}.app"

echo "==> Building frontend..."
cd frontend && npm run build --silent && cd ..
rm -rf internal/server/frontend/dist
cp -r frontend/dist internal/server/frontend/dist

echo "==> Building binary..."
CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags "desktop production" -o "${APP_NAME}" ./cmd/crawlobserver

echo "==> Creating ${BUNDLE}..."
rm -rf "${BUNDLE}"
mkdir -p "${BUNDLE}/Contents/MacOS"
mkdir -p "${BUNDLE}/Contents/Resources"

cp build/darwin/Info.plist "${BUNDLE}/Contents/Info.plist"
cp "${APP_NAME}" "${BUNDLE}/Contents/MacOS/${APP_NAME}"
cp build/darwin/iconfile.icns "${BUNDLE}/Contents/Resources/iconfile.icns"

echo "==> Done! Run with: open ${BUNDLE}"
