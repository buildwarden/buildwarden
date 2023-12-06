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

help_exit() {
  code="$1"
  myname="$(basename "$0")"
  
  >&2 echo "USAGE: $myname [COMMAND...]"
  >&2 echo 'COMMANDS:'
  >&2 echo '    build, fmt, lint, test, tidy'
  >&2 echo 'EXAMPLES:'
  >&2 echo '    Just compile:'
  >&2 echo "        $myname build"
  >&2 echo '    Do many things:'
  >&2 echo "        $myname tidy fmt build test"

  exit "$code"
}

if [ "$*" = '' ]; then
  help_exit 1
fi

for arg in "$@"; do
  if [ "$arg" = '-h' ] || [ "$arg" = '--help' ]; then
    help_exit 0
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
