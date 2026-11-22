#!/bin/sh
# The conformance workflow's pin job (Decision 7 of docs/design/seam-specs.md).
# On a pull request and on main, each tools/conformance/<directory>/go.mod
# must name its own specification at a release tag, no other module of
# buildwarden/buildwarden-specs but the harness, whose line is a release tag
# too, and no replace line; and make pin-specs, given the versions the six
# files name, must change no file, which holds each go.mod and go.sum to what
# that release names. A branch in progress may pin an unreleased commit, so
# the workflow does not run this on a branch push. A pull request to a
# program trunk (GITHUB_BASE_REF program/...) may also pin a pre-release,
# vX.Y.Z-dev.N or -rc.N, of a specification and of the harness; one to main
# may not (FLOW-20).
#
#   tools/conformance/check-pins.sh     # from the repository root
set -eu

repo=github.com/buildwarden/buildwarden-specs
harness=$repo/conformance
dirs="bootstrap-config collector-inputs collector-outputs executor-initialize relay-claim relay-guest-api"
release_re='^v[0-9]+\.[0-9]+\.[0-9]+$'
case "${GITHUB_BASE_REF:-}" in
program/*) release_re='^v[0-9]+\.[0-9]+\.[0-9]+(-(dev|rc)\.[0-9]+)?$' ;;
esac
fail=0
specs=""

bad() {
	echo "pin: $*" >&2
	fail=1
}

is_release() { echo "$1" | grep -Eq "$release_re"; }

for d in $dirs; do
	mod="tools/conformance/$d/go.mod"
	if [ ! -f "$mod" ]; then
		bad "$mod is missing"
		continue
	fi
	json=$(go mod edit -json "$mod")
	replaces=$(echo "$json" | jq -r '.Replace // [] | .[] | .Old.Path')
	[ -z "$replaces" ] || bad "$mod carries a replace line for $replaces"
	pin=""
	hv=""
	for line in $(echo "$json" | jq -r '.Require // [] | .[] | "\(.Path)@\(.Version)"'); do
		p=${line%@*}
		v=${line##*@}
		case "$p" in
		"$repo/$d") pin=$v ;;
		"$harness") hv=$v ;;
		"$repo"/*) bad "$mod requires a second module of the specs repository, $p" ;;
		esac
	done
	if [ -z "$pin" ]; then
		bad "$mod does not require $repo/$d"
	elif ! is_release "$pin"; then
		bad "$mod pins $d at $pin, not a release tag"
	fi
	if [ -z "$hv" ]; then
		bad "$mod does not require the harness, $harness"
	elif ! is_release "$hv"; then
		bad "$mod names the harness at $hv, not a release tag"
	fi
	specs="$specs $d@$pin"
done
[ "$fail" = 0 ] || exit 1

# What the six modules and the page hold now, compared with what make
# pin-specs writes for the same versions, whether or not it is committed.
files="docs/book/src/concepts/seams.md"
for d in $dirs; do files="$files tools/conformance/$d/go.mod tools/conformance/$d/go.sum"; done
before=$(mktemp -d)
trap 'rm -rf "$before"' EXIT
for f in $files; do
	mkdir -p "$before/$(dirname "$f")"
	cp "$f" "$before/$f"
done
make --no-print-directory pin-specs SPECS="${specs# }"
changed=0
for f in $files; do
	diff -u "$before/$f" "$f" >&2 || changed=1
done
if [ "$changed" != 0 ]; then
	echo "pin: make pin-specs SPECS=\"${specs# }\" changed what the modules name (above)" >&2
	exit 1
fi
echo "pin: the six modules name releases, as make pin-specs writes them"
