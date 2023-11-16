#!/bin/sh

set -o errexit
set -o nounset

projroot="$(cd "$(dirname "$0")" && pwd)"
cd "$projroot" || exit 1

test_simple() {
  builddir="$projroot/build"
  mkdir -p "$builddir"
  (
    cd "$builddir" || exit 1
    >&2 echo "Building at $builddir"
    go build "$projroot/cmd/warden/"
    ./warden build "$projroot/buildctx" -f Dockerfile.simple
  )
}

for arg in "$@"; do
  if [ "$arg" = '-h' ] || [ "$arg" = '--help' ]; then
    >&2 echo "USAGE: $(basename "$0") [COMMAND...]"
    >&2 echo 'COMMANDS:'
    >&2 echo '    build, fmt, lint, test, tidy'
    exit 0
  fi
done

while [ "$#" -gt 0 ]; do
  case "$1" in
    build) go install ./... ;;
    fmt)   gofmt -s -w . ;;
    lint)  golangci-lint run
           shellcheck build.sh ;;
    test)  test_simple ;;
    tidy)  go mod tidy ;;
  esac
  shift
done
