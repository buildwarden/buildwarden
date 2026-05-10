#!/usr/bin/env bash
# Integration test: validates warden builds produce correct ledgers.
# Run: make integration-test
# Requires: a working container runtime (finch/docker/podman)
set -euo pipefail

WARDEN="./warden"
PASS=0
FAIL=0

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
bold()  { printf '\033[1m%s\033[0m\n' "$*"; }

assert_grep() {
    local desc="$1" pattern="$2" file="$3"
    if grep -qE "$pattern" "$file"; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc (pattern: $pattern)"
        FAIL=$((FAIL + 1))
    fi
}

assert_no_grep() {
    local desc="$1" pattern="$2" file="$3"
    if ! grep -qE "$pattern" "$file"; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc (unexpected match: $pattern)"
        FAIL=$((FAIL + 1))
    fi
}

extract_ledger_dir() {
    # Strip ANSI codes then extract path after "Ledger:"
    sed 's/\x1b\[[0-9;]*m//g' | grep "Ledger:" | awk '{print $NF}'
}

# ─── Build warden ───────────────────────────────────────────────────────────

bold "Building warden..."
make build

# ─── Test 1: Self-build ─────────────────────────────────────────────────────

bold ""
bold "━━━ Test 1: Self-build (COPY provenance + Go modules + artifact) ━━━"

BUILD_OUT=$($WARDEN build . 2>&1)
LEDGER_DIR=$(echo "$BUILD_OUT" | extract_ledger_dir)
if [ -z "$LEDGER_DIR" ]; then
    red "FATAL: self-build failed"
    echo "$BUILD_OUT" | tail -10
    exit 1
fi

INSPECT=$($WARDEN inspect "$LEDGER_DIR/ledger" 2>&1 | sed 's/\x1b\[[0-9;]*m//g')
INSPECT_FILE=$(mktemp)
echo "$INSPECT" > "$INSPECT_FILE"

bold "  Ledger integrity:"
assert_grep "All signatures valid" "All .* signatures valid" "$INSPECT_FILE"
assert_grep "All channels closed" "All channels closed" "$INSPECT_FILE"

bold "  COPY provenance (source files fetched via relay):"
assert_grep "go.mod via relay" "GET http://cwd/go.mod" "$INSPECT_FILE"
assert_grep "go.sum via relay" "GET http://cwd/go.sum" "$INSPECT_FILE"
assert_grep "cmd/warden/main.go via relay" "GET http://cwd/cmd/warden/main.go" "$INSPECT_FILE"
assert_grep "relay/relay.go via relay" "GET http://cwd/relay/relay.go" "$INSPECT_FILE"
assert_grep "relay/ledger.go via relay" "GET http://cwd/relay/ledger.go" "$INSPECT_FILE"
assert_grep "orchestrator via relay" "GET http://cwd/internal/orchestrator/" "$INSPECT_FILE"

bold "  Security:"
assert_no_grep "No .git leaked" "cwd/\.git/" "$INSPECT_FILE"
assert_no_grep "No .warden leaked" "cwd/\.warden/" "$INSPECT_FILE"

bold "  Expected external sources:"
assert_grep "Docker Hub registry" "registry-1.docker.io" "$INSPECT_FILE"
assert_grep "Golang base image" "library/golang" "$INSPECT_FILE"
assert_grep "Debian package repos" "deb.debian.org" "$INSPECT_FILE"

bold "  Artifact:"
assert_grep "Warden binary posted" "ARTIFACT.*artifacts/warden" "$INSPECT_FILE"

# Static hash check: go.mod size should match local file
LOCAL_GOMOD_SIZE=$(wc -c < go.mod | tr -d ' ')
assert_grep "go.mod size matches local" \
    "GET http://cwd/go.mod \(${LOCAL_GOMOD_SIZE} bytes\)" "$INSPECT_FILE"

rm -f "$INSPECT_FILE"
green "  Ledger: $LEDGER_DIR"

# ─── Test 2: Simple demo ────────────────────────────────────────────────────

bold ""
bold "━━━ Test 2: Simple demo (pip build + deterministic wheel artifact) ━━━"

BUILD_OUT=$($WARDEN build examples/Dockerfile.simple 2>&1)
LEDGER_DIR=$(echo "$BUILD_OUT" | extract_ledger_dir)
if [ -z "$LEDGER_DIR" ]; then
    red "FATAL: simple build failed"
    echo "$BUILD_OUT" | tail -10
    exit 1
fi

INSPECT=$($WARDEN inspect "$LEDGER_DIR/ledger" 2>&1 | sed 's/\x1b\[[0-9;]*m//g')
INSPECT_FILE=$(mktemp)
echo "$INSPECT" > "$INSPECT_FILE"

bold "  Ledger integrity:"
assert_grep "All signatures valid" "All .* signatures valid" "$INSPECT_FILE"
assert_grep "All channels closed" "All channels closed" "$INSPECT_FILE"

bold "  Expected sources:"
assert_grep "Docker Hub registry" "registry-1.docker.io" "$INSPECT_FILE"
assert_grep "Python base image" "library/python" "$INSPECT_FILE"
assert_grep "PyPI package index" "pypi.org" "$INSPECT_FILE"
assert_grep "Python hosted files" "files.pythonhosted.org" "$INSPECT_FILE"
assert_grep "Debian repos" "deb.debian.org" "$INSPECT_FILE"

bold "  Deterministic artifact (pinned version):"
assert_grep "requests 2.32.3 sdist fetched" "requests-2.32.3.tar.gz" "$INSPECT_FILE"
assert_grep "Wheel artifact posted" "ARTIFACT.*requests-2.32.3-py3-none-any.whl" "$INSPECT_FILE"
# The wheel size is deterministic for this pure-python package with SOURCE_DATE_EPOCH
assert_grep "Wheel size is 65027 bytes" "65027 bytes" "$INSPECT_FILE"

rm -f "$INSPECT_FILE"
green "  Ledger: $LEDGER_DIR"

# ─── Summary ────────────────────────────────────────────────────────────────

echo
bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
bold "  Results: $PASS passed, $FAIL failed"
bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ "$FAIL" -gt 0 ]; then
    red "  INTEGRATION TEST FAILED"
    exit 1
fi
green "  ALL INTEGRATION TESTS PASSED"
