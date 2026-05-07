#!/usr/bin/env bash
# CLUE CODE — install script dry-run tests
#
# Verifies OS/arch detection and dry-run mode without downloading anything.
# Run: bash scripts/install_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="${SCRIPT_DIR}/install.sh"

PASS=0
FAIL=0

assert_contains() {
  local label="$1" output="$2" expected="$3"
  if echo "$output" | grep -qF -- "$expected"; then
    echo "  PASS: ${label}"
    ((PASS++)) || true
  else
    echo "  FAIL: ${label}"
    echo "    Expected to find: ${expected}"
    echo "    In output: ${output}"
    ((FAIL++)) || true
  fi
}

assert_exit() {
  local label="$1" code="$2" expected="$3"
  if [[ "$code" -eq "$expected" ]]; then
    echo "  PASS: ${label} (exit ${code})"
    ((PASS++)) || true
  else
    echo "  FAIL: ${label} (exit ${code}, expected ${expected})"
    ((FAIL++)) || true
  fi
}

echo "=== CLUE CODE install.sh dry-run tests ==="
echo ""

# ---------------------------------------------------------------------------
# Test 1: --dry-run detects current OS and arch
# ---------------------------------------------------------------------------
echo "Test 1: --dry-run OS/arch detection"
output="$(bash "${INSTALL_SH}" --dry-run --version v1.0.0 2>&1)" || true
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64)          ARCH="amd64" ;;
  arm64|aarch64)   ARCH="arm64" ;;
  *)               ARCH="$ARCH_RAW" ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) echo "  SKIP: unsupported OS ${OS}"; PASS=$((PASS+1)); ;;
esac
assert_contains "dry-run mentions OS" "$output" "$OS"
assert_contains "dry-run mentions arch" "$output" "$ARCH"
assert_contains "dry-run mentions version" "$output" "v1.0.0"
assert_contains "dry-run does not install" "$output" "dry-run"

# ---------------------------------------------------------------------------
# Test 2: --dry-run exit code is 0
# ---------------------------------------------------------------------------
echo "Test 2: --dry-run exit code"
set +e
bash "${INSTALL_SH}" --dry-run --version v1.0.0 >/dev/null 2>&1
code=$?
set -e
assert_exit "--dry-run exits 0" "$code" 0

# ---------------------------------------------------------------------------
# Test 3: --help exits 0 and prints usage
# ---------------------------------------------------------------------------
echo "Test 3: --help"
help_output="$(bash "${INSTALL_SH}" --help 2>&1)"
set +e
bash "${INSTALL_SH}" --help >/dev/null 2>&1
help_code=$?
set -e
assert_exit "--help exits 0" "$help_code" 0
assert_contains "--help mentions --dry-run" "$help_output" "--dry-run"
assert_contains "--help mentions --version" "$help_output" "--version"

# ---------------------------------------------------------------------------
# Test 4: Unknown flag exits non-zero
# ---------------------------------------------------------------------------
echo "Test 4: unknown flag exits non-zero"
set +e
bash "${INSTALL_SH}" --unknown-flag >/dev/null 2>&1
bad_code=$?
set -e
if [[ "$bad_code" -ne 0 ]]; then
  echo "  PASS: unknown flag exits non-zero (exit ${bad_code})"
  ((PASS++)) || true
else
  echo "  FAIL: unknown flag should exit non-zero but got 0"
  ((FAIL++)) || true
fi

# ---------------------------------------------------------------------------
# Test 5: --dry-run with --install-dir mentions the custom path
# ---------------------------------------------------------------------------
echo "Test 5: --install-dir appears in dry-run output"
TMPDIR_TEST="$(mktemp -d)"
output5="$(bash "${INSTALL_SH}" --dry-run --version v1.0.0 --install-dir "$TMPDIR_TEST" 2>&1)" || true
assert_contains "custom install-dir in output" "$output5" "$TMPDIR_TEST"
rm -rf "$TMPDIR_TEST"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi
exit 0
