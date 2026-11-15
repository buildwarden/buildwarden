#!/usr/bin/env bash
# Integration test: validates warden builds produce correct ledgers.
# Run: make integration-test
# Requires: a working container runtime (finch/docker/podman)
set -euo pipefail

WARDEN="${WARDEN:-./dist/warden}"   # make build writes dist/warden
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

# The build's last line is "warden: build SUCCEEDED (output: <dir>)".
extract_output_dir() {
    sed 's/\x1b\[[0-9;]*m//g' | sed -n 's/.*build SUCCEEDED (output: \(.*\))$/\1/p' | tail -1
}

# ─── Build warden ───────────────────────────────────────────────────────────

bold "Building warden..."
make build

# Absolute: the runtime cases below run warden from other directories.
WARDEN="$(cd "$(dirname "$WARDEN")" && pwd)/$(basename "$WARDEN")"
REPO="$PWD"

# ─── Test 0: an unsupported container runtime is refused, not run ───────────

bold ""
bold "━━━ Test 0: [runtime] cli, WARDEN_CTR_CLI and --runtime name finch, docker or podman ━━━"

# A project that ships a program and names it as the container runtime. The
# program leaves a marker when it is run. A home of its own keeps the caller's
# config file and cache out of these cases.
RT_PROJECT=$(mktemp -d)
RT_HOME=$(mktemp -d)
RT_MARKER="$RT_HOME/ran"
printf 'FROM alpine\n' > "$RT_PROJECT/Dockerfile"
printf '#!/bin/sh\necho ran >> "%s"\n' "$RT_MARKER" > "$RT_PROJECT/rt"
chmod +x "$RT_PROJECT/rt"

# rt_warden [VAR=value ...] <warden arguments>: runs warden in the project;
# its output is in RT_OUT and its exit code in RT_RC.
rt_warden() {
    local env=()
    while [ $# -gt 0 ] && [[ "$1" == *=* ]]; do
        env+=("$1")
        shift
    done
    RT_RC=0
    RT_OUT=$(cd "$RT_PROJECT" && env -u WARDEN_CTR_CLI -u WARDEN_DRIVER \
        HOME="$RT_HOME" USERPROFILE="$RT_HOME" ${env[@]+"${env[@]}"} \
        "$WARDEN" "$@" 2>&1) || RT_RC=$?
}

rt_result() {
    local desc="$1" ok="$2"
    if [ "$ok" = yes ]; then
        green "  ✓ $desc"
        PASS=$((PASS + 1))
    else
        red "  ✗ $desc (exit $RT_RC)"
        echo "$RT_OUT" | tail -5
        FAIL=$((FAIL + 1))
    fi
    rm -rf "$RT_MARKER" "$RT_PROJECT/warden-output"
}

# The command exits 1 with a fix, the shipped program was not run, and no
# output directory was made.
assert_runtime_refused() {
    local desc="$1" ok=no
    if [ -e "$RT_MARKER" ]; then
        desc="$desc: the program was run"
    fi
    if [ "$RT_RC" -eq 1 ] && grep -q 'To fix:' <<<"$RT_OUT" &&
        [ ! -e "$RT_MARKER" ] && [ ! -e "$RT_PROJECT/warden-output" ]; then
        ok=yes
    fi
    rt_result "$desc" "$ok"
}

# The command exits 0, the file's unsupported value notwithstanding, and
# its output matches the pattern when one is given.
assert_runtime_unchecked() {
    local desc="$1" pattern="${2:-}" ok=no
    if [ "$RT_RC" -eq 0 ] && grep -qE "$pattern" <<<"$RT_OUT"; then
        ok=yes
    fi
    rt_result "$desc" "$ok"
}

bold "  A program the project ships, named by its warden.toml:"
printf '[runtime]\ncli = "./rt"\n' > "$RT_PROJECT/warden.toml"
for cmd in build clean doctor; do
    rt_warden "$cmd"
    assert_runtime_refused "warden $cmd refuses cli = \"./rt\" and does not run it"
done
rt_warden clean --driver vz
assert_runtime_refused "warden clean --driver vz refuses it and does not run it"

printf '[Runtime]\nCLI = "./rt"\n' > "$RT_PROJECT/warden.toml"
rt_warden clean
assert_runtime_refused "warden clean refuses [Runtime] CLI = \"./rt\" and does not run it"

printf '[runtime]\ncli = "./rt"\ndriver = "qemu"\n' > "$RT_PROJECT/warden.toml"
rt_warden clean
assert_runtime_refused "warden clean refuses it beside driver = \"qemu\" and does not run it"

bold "  A program named by the flag or the variable:"
rm -f "$RT_PROJECT/warden.toml"
rt_warden doctor --runtime "$RT_PROJECT/rt"
assert_runtime_refused "warden doctor refuses the program's path in --runtime"
rt_warden WARDEN_CTR_CLI="$RT_PROJECT/rt" doctor
assert_runtime_refused "warden doctor refuses the program's path in WARDEN_CTR_CLI"

bold "  The value in effect is the one checked:"
printf '[runtime]\ncli = "./rt"\n' > "$RT_PROJECT/warden.toml"
rt_warden doctor --runtime docker
assert_runtime_unchecked "--runtime docker replaces the file's value" \
    '^container runtime: docker$'
rt_warden WARDEN_CTR_CLI=docker doctor
assert_runtime_unchecked "WARDEN_CTR_CLI=docker replaces the file's value" \
    '^container runtime: docker$'

bold "  A run that does not use the container runtime does not check it:"
rt_warden setup
assert_runtime_unchecked "warden setup ignores the file's value"
# On a Windows host this would clean the host's real Hyper-V leftovers.
case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*) ;;
*)
    rt_warden clean --driver hyperv
    assert_runtime_unchecked "warden clean --driver hyperv ignores the file's value"
    ;;
esac

rm -rf "$RT_PROJECT" "$RT_HOME"

# ─── Test 1: Self-build ─────────────────────────────────────────────────────

bold ""
bold "━━━ Test 1: Self-build (COPY provenance + Go modules + artifact) ━━━"

BUILD_OUT=$($WARDEN build -o /tmp/warden-integ-self . 2>&1) || true  # a failed build is reported below
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
assert_grep "relay.go via relay" "CONTEXT GET /relay/relay.go" "$INSPECT_FILE"
assert_grep "ledger.go via relay" "CONTEXT GET /relay/ledger.go" "$INSPECT_FILE"
assert_grep "container.go via relay" "CONTEXT GET /driver/container/container.go" "$INSPECT_FILE"

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

BUILD_OUT=$($WARDEN build -o /tmp/warden-integ-simple examples/Dockerfile.simple 2>&1) || true  # a failed build is reported below
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

bold "  Recorded inputs (ORCH-02):"
if cmp -s examples/Dockerfile.simple "$OUTPUT_DIR/inputs/Containerfile"; then
    green "  ✓ inputs/Containerfile is the submitted file"
    PASS=$((PASS + 1))
else
    red "  ✗ inputs/Containerfile missing or differs from examples/Dockerfile.simple"
    FAIL=$((FAIL + 1))
fi
if [ -f "$OUTPUT_DIR/inputs/build.sh" ]; then
    green "  ✓ inputs/build.sh recorded"
    PASS=$((PASS + 1))
else
    red "  ✗ inputs/build.sh missing"
    FAIL=$((FAIL + 1))
fi
if [ ! -e "$OUTPUT_DIR/inputs/Dockerfile" ]; then
    green "  ✓ no inputs/Dockerfile"
    PASS=$((PASS + 1))
else
    red "  ✗ inputs/Dockerfile written; the submitted file keeps a fixed name"
    FAIL=$((FAIL + 1))
fi

rm -f "$INSPECT_FILE"
green "  Output: $OUTPUT_DIR"

# ─── Test 3: npm (different ecosystem) ─────────────────────────────────────

bold ""
bold "━━━ Test 3: npm install (Node ecosystem), the runtime named by a project's warden.toml ━━━"

# A supported runtime named by a project's file is still built with: this
# build runs from a directory whose warden.toml names the runtime that
# detection finds on this host. The caller's WARDEN_CTR_CLI and WARDEN_DRIVER
# outrank the file, so both commands run without them: with either set, the
# build would pass without reading the file's value.
NAMED_RUNTIME=$(env -u WARDEN_CTR_CLI -u WARDEN_DRIVER "$WARDEN" doctor 2>/dev/null |
    sed -n 's/^container runtime: //p') || true
NAMED_PROJECT=$(mktemp -d)
printf '[runtime]\ncli = "%s"\n' "$NAMED_RUNTIME" > "$NAMED_PROJECT/warden.toml"
BUILD_OUT=$(cd "$NAMED_PROJECT" && env -u WARDEN_CTR_CLI -u WARDEN_DRIVER "$WARDEN" build -o /tmp/warden-integ-npm "$REPO/examples/Dockerfile.npm" 2>&1) || true  # a failed build is reported below
rm -rf "$NAMED_PROJECT"
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

bold "  The runtime the project's file named:"
if [ -n "$NAMED_RUNTIME" ]; then
    green "  ✓ built with cli = \"$NAMED_RUNTIME\" in the project's warden.toml"
    PASS=$((PASS + 1))
else
    red "  ✗ warden doctor reported no container runtime to name in warden.toml"
    FAIL=$((FAIL + 1))
fi

bold "  Expected sources:"
assert_grep "npm registry" "registry.npmjs.org" "$INSPECT_FILE"
assert_grep "lodash package" "lodash" "$INSPECT_FILE"

rm -f "$INSPECT_FILE"
green "  Output: $OUTPUT_DIR"

# ─── Cleanup ───────────────────────────────────────────────────────────────

# Keep the outputs of a failed run for inspection (CI uploads them).
if [ "$FAIL" -eq 0 ]; then
    rm -rf /tmp/warden-integ-self /tmp/warden-integ-simple /tmp/warden-integ-npm
fi

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
