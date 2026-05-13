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

extract_output_dir() {
    sed 's/\x1b\[[0-9;]*m//g' | grep "Output:" | awk '{print $NF}'
}

# ─── Build warden ───────────────────────────────────────────────────────────

bold "Building warden..."
make build

# ─── Test 1: Self-build ─────────────────────────────────────────────────────

bold ""
bold "━━━ Test 1: Self-build (COPY provenance + Go modules + artifact) ━━━"

BUILD_OUT=$($WARDEN build -o /tmp/warden-integ-self . 2>&1)
OUTPUT_DIR=$(echo "$BUILD_OUT" | extract_output_dir)
if [ -z "$OUTPUT_DIR" ] || [ ! -d "$OUTPUT_DIR" ]; then
    red "FATAL: self-build failed"
    echo "$BUILD_OUT" | tail -10
    exit 1
fi

INSPECT=$($WARDEN inspect "$OUTPUT_DIR" 2>&1 | sed 's/\x1b\[[0-9;]*m//g')
INSPECT_FILE=$(mktemp)
echo "$INSPECT" > "$INSPECT_FILE"

bold "  Ledger integrity:"
assert_grep "All signatures valid" "All .* signatures valid" "$INSPECT_FILE"
assert_grep "All channels closed" "All channels closed" "$INSPECT_FILE"

bold "  Environment record:"
assert_grep "Environment entry" "ENVIRONMENT golang:" "$INSPECT_FILE"

bold "  COPY provenance (source files fetched via relay):"
assert_grep "go.mod via relay" "CONTEXT GET /go.mod" "$INSPECT_FILE"
assert_grep "go.sum via relay" "CONTEXT GET /go.sum" "$INSPECT_FILE"
assert_grep "main.go via relay" "CONTEXT GET /cmd/warden/main.go" "$INSPECT_FILE"
assert_grep "relay.go via relay" "CONTEXT GET /cmd/relay/relay.go" "$INSPECT_FILE"
assert_grep "ledger.go via relay" "CONTEXT GET /cmd/relay/ledger.go" "$INSPECT_FILE"
assert_grep "orchestrator via relay" "CONTEXT GET /cmd/warden/orchestrator" "$INSPECT_FILE"

bold "  Security:"
assert_no_grep "No .git leaked" "CONTEXT.*\.git/" "$INSPECT_FILE"
assert_no_grep "No .warden leaked" "CONTEXT.*\.warden/" "$INSPECT_FILE"

bold "  Artifacts:"
assert_grep "Warden binary posted" "ARTIFACT.*warden" "$INSPECT_FILE"
assert_grep "Relay binary posted" "ARTIFACT.*relay" "$INSPECT_FILE"

rm -f "$INSPECT_FILE"
green "  Output: $OUTPUT_DIR"

# ─── Test 2: Simple demo ────────────────────────────────────────────────────

bold ""
bold "━━━ Test 2: Simple demo (pip build + deterministic wheel artifact) ━━━"

BUILD_OUT=$($WARDEN build -o /tmp/warden-integ-simple examples/Dockerfile.simple 2>&1)
OUTPUT_DIR=$(echo "$BUILD_OUT" | extract_output_dir)
if [ -z "$OUTPUT_DIR" ] || [ ! -d "$OUTPUT_DIR" ]; then
    red "FATAL: simple build failed"
    echo "$BUILD_OUT" | tail -10
    exit 1
fi

INSPECT=$($WARDEN inspect "$OUTPUT_DIR" 2>&1 | sed 's/\x1b\[[0-9;]*m//g')
INSPECT_FILE=$(mktemp)
echo "$INSPECT" > "$INSPECT_FILE"

bold "  Ledger integrity:"
assert_grep "All signatures valid" "All .* signatures valid" "$INSPECT_FILE"
assert_grep "All channels closed" "All channels closed" "$INSPECT_FILE"

bold "  Environment record:"
assert_grep "Python base image" "ENVIRONMENT python:" "$INSPECT_FILE"

bold "  Expected sources:"
assert_grep "PyPI package index" "pypi.org" "$INSPECT_FILE"
assert_grep "Python hosted files" "files.pythonhosted.org" "$INSPECT_FILE"

bold "  Deterministic artifact (pinned version):"
assert_grep "requests sdist fetched" "requests-2.32.3.tar.gz" "$INSPECT_FILE"
assert_grep "Wheel artifact posted" "ARTIFACT.*requests-2.32.3-py3-none-any.whl" "$INSPECT_FILE"
assert_grep "Wheel size is 65027 bytes" "65027 bytes" "$INSPECT_FILE"

bold "  JSON output:"
JSON_OUT=$($WARDEN inspect --json "$OUTPUT_DIR" 2>&1)
if echo "$JSON_OUT" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d['summary']['valid']" 2>/dev/null; then
    green "  ✓ JSON valid and ledger passes"
    PASS=$((PASS + 1))
else
    red "  ✗ JSON output invalid or ledger failed"
    FAIL=$((FAIL + 1))
fi

rm -f "$INSPECT_FILE"
green "  Output: $OUTPUT_DIR"

# ─── Test 3: npm (different ecosystem) ─────────────────────────────────────

bold ""
bold "━━━ Test 3: npm install (Node ecosystem) ━━━"

BUILD_OUT=$($WARDEN build -o /tmp/warden-integ-npm examples/Dockerfile.npm 2>&1)
OUTPUT_DIR=$(echo "$BUILD_OUT" | extract_output_dir)
if [ -z "$OUTPUT_DIR" ] || [ ! -d "$OUTPUT_DIR" ]; then
    red "FATAL: npm build failed"
    echo "$BUILD_OUT" | tail -10
    exit 1
fi

INSPECT=$($WARDEN inspect "$OUTPUT_DIR" 2>&1 | sed 's/\x1b\[[0-9;]*m//g')
INSPECT_FILE=$(mktemp)
echo "$INSPECT" > "$INSPECT_FILE"

bold "  Ledger integrity:"
assert_grep "All signatures valid" "All .* signatures valid" "$INSPECT_FILE"
assert_grep "All channels closed" "All channels closed" "$INSPECT_FILE"

bold "  Expected sources:"
assert_grep "npm registry" "registry.npmjs.org" "$INSPECT_FILE"
assert_grep "lodash package" "lodash" "$INSPECT_FILE"

rm -f "$INSPECT_FILE"
green "  Output: $OUTPUT_DIR"

# ─── Cleanup ───────────────────────────────────────────────────────────────

rm -rf /tmp/warden-integ-self /tmp/warden-integ-simple /tmp/warden-integ-npm

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
