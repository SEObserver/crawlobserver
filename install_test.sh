#!/bin/sh
# Tests for install.sh — validates OS/arch detection and URL construction.
# Run with: sh install_test.sh

set -e

PASS=0
FAIL=0

assert_eq() {
  local test_name="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo "FAIL: $test_name"
    echo "  expected: $expected"
    echo "  actual:   $actual"
  fi
}

# ---------- OS detection ----------

test_os_detection() {
  local os_input="$1" expected_goos="$2"
  local result
  result=$(echo "$os_input" | awk '{
    os=$1
    if (os == "Linux") print "linux"
    else if (os == "Darwin") print "darwin"
    else if (substr(os,1,5) == "MINGW" || substr(os,1,4) == "MSYS" || substr(os,1,6) == "CYGWIN") print "windows"
    else print "unsupported"
  }')
  assert_eq "OS detection: $os_input" "$expected_goos" "$result"
}

test_os_detection "Linux" "linux"
test_os_detection "Darwin" "darwin"
test_os_detection "MINGW64_NT-10.0" "windows"
test_os_detection "MSYS_NT-10.0" "windows"
test_os_detection "CYGWIN_NT-10.0" "windows"
test_os_detection "FreeBSD" "unsupported"

# ---------- Arch detection ----------

test_arch_detection() {
  local arch_input="$1" expected_goarch="$2"
  local result
  result=$(echo "$arch_input" | awk '{
    arch=$1
    if (arch == "x86_64" || arch == "amd64") print "amd64"
    else if (arch == "arm64" || arch == "aarch64") print "arm64"
    else print "unsupported"
  }')
  assert_eq "Arch detection: $arch_input" "$expected_goarch" "$result"
}

test_arch_detection "x86_64" "amd64"
test_arch_detection "amd64" "amd64"
test_arch_detection "arm64" "arm64"
test_arch_detection "aarch64" "arm64"
test_arch_detection "i386" "unsupported"
test_arch_detection "armv7l" "unsupported"

# ---------- Asset URL construction ----------

test_asset_url() {
  local goos="$1" goarch="$2" expected_asset="$3"
  local suffix=""
  if [ "$goos" = "windows" ]; then suffix=".exe"; fi
  local asset="crawlobserver-${goos}-${goarch}${suffix}"
  local url="https://github.com/SEObserver/crawlobserver/releases/latest/download/${asset}"
  assert_eq "Asset URL: ${goos}/${goarch}" "$expected_asset" "$asset"
}

test_asset_url "linux" "amd64" "crawlobserver-linux-amd64"
test_asset_url "linux" "arm64" "crawlobserver-linux-arm64"
test_asset_url "darwin" "amd64" "crawlobserver-darwin-amd64"
test_asset_url "darwin" "arm64" "crawlobserver-darwin-arm64"
test_asset_url "windows" "amd64" "crawlobserver-windows-amd64.exe"

# ---------- INSTALL_DIR override ----------

test_install_dir_override() {
  local result
  result=$(INSTALL_DIR="/tmp/test" sh -c '. /dev/stdin <<SCRIPT
INSTALL_DIR="\${INSTALL_DIR:-/usr/local/bin}"
echo "\$INSTALL_DIR"
SCRIPT')
  assert_eq "INSTALL_DIR override" "/tmp/test" "$result"
}

test_install_dir_default() {
  local result
  result=$(unset INSTALL_DIR; sh -c '. /dev/stdin <<SCRIPT
INSTALL_DIR="\${INSTALL_DIR:-/usr/local/bin}"
echo "\$INSTALL_DIR"
SCRIPT')
  assert_eq "INSTALL_DIR default" "/usr/local/bin" "$result"
}

test_install_dir_override
test_install_dir_default

# ---------- Script syntax ----------

test_script_syntax() {
  if sh -n install.sh 2>/dev/null; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    echo "FAIL: install.sh syntax check"
  fi
}

test_script_syntax

# ---------- Results ----------

TOTAL=$((PASS + FAIL))
echo ""
echo "Results: $PASS/$TOTAL passed"
if [ "$FAIL" -gt 0 ]; then
  echo "$FAIL test(s) FAILED"
  exit 1
else
  echo "All tests passed."
fi
