#!/bin/bash
set -e

APP_NAME="CrawlObserver"
BUNDLE="build/${APP_NAME}.app"

echo "==> Building frontend..."
cd frontend && npm run build --silent && cd ..
rm -rf internal/server/frontend/dist
cp -r frontend/dist internal/server/frontend/dist

echo "==> Building binary..."
go build -tags desktop -o "${APP_NAME}" ./cmd/crawlobserver

echo "==> Creating ${BUNDLE}..."
rm -rf "${BUNDLE}"
mkdir -p "${BUNDLE}/Contents/MacOS"
mkdir -p "${BUNDLE}/Contents/Resources"

cp build/darwin/Info.plist "${BUNDLE}/Contents/Info.plist"
cp "${APP_NAME}" "${BUNDLE}/Contents/MacOS/${APP_NAME}"
cp build/darwin/iconfile.icns "${BUNDLE}/Contents/Resources/iconfile.icns"

echo "==> Creating DMG..."
DMG_NAME="${APP_NAME}.dmg"
DMG_STAGING="build/dmg-staging"
rm -rf "${DMG_STAGING}" "build/${DMG_NAME}"
mkdir -p "${DMG_STAGING}"
cp -R "${BUNDLE}" "${DMG_STAGING}/"
ln -s /Applications "${DMG_STAGING}/Applications"
hdiutil create "build/${DMG_NAME}" -volname "${APP_NAME}" -srcfolder "${DMG_STAGING}" -format UDZO -ov
rm -rf "${DMG_STAGING}"

echo "==> Done! DMG: build/${DMG_NAME}"
echo "    Or run directly: open ${BUNDLE}"
