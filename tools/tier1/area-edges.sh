#!/bin/sh
# area-edges.sh prints this repository's dependency edges, one "<dir> <dir>"
# per line: a package's directory, and the directory of a package it or its
# tests depend on, directly or not, both relative to the repository root. It
# is the command the profile's Areas entry names (PROFILE-01): the loop
# compares the area map's `reaches` with these lines, because wf itself runs
# no toolchain (LOOP-08):
#
#   tools/tier1/area-edges.sh | dist/wf areas -check -state .dev/workflow -repo . -edges -
#
# It is not a check. Tier 1 does not call it, and it exits non-zero only when
# a `go list` run fails.
set -eu

root=$(cd "$(dirname "$0")/../.." && go list -m -f '{{.Dir}}')
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Every module, for every platform the product builds on: an import behind a
# build tag is still a dependency between two areas.
for module in . tools/tier1 tools/workflow; do
	for os in linux darwin windows; do
		for arch in amd64 arm64; do
			# cgo is on only where driver/vz's cgo files build; every
			# other target compiles its stub, with cgo on or off.
			cgo=0
			if [ "$os/$arch" = darwin/arm64 ]; then
				cgo=1
			fi
			# To a file and not a pipe: a failed run must fail the script,
			# and sh has no pipefail.
			(cd "$root/$module" && GOOS=$os GOARCH=$arch CGO_ENABLED=$cgo \
				go list -deps -test \
				-f '{{.Dir}}{{"\t"}}{{.ImportPath}}{{"\t"}}{{join .Deps ","}}' ./...) \
				>>"$tmp/packages"
		done
	done
done

# A test variant is listed as "pkg [pkg.test]"; it is pkg's directory with
# its tests' dependencies, so the suffix is dropped on both sides. Packages
# outside the repository (the standard library, other modules) have no
# directory under the root and make no edge.
awk -F '\t' -v root="$root/" '
function name(p) { sub(/ \[.*\]$/, "", p); return p }
function rel(d) { return d "/" == root ? "." : substr(d, length(root) + 1) }
index($1 "/", root) == 1 {
	dir[name($2)] = $1
	deps[NR] = $3
	from[NR] = $1
}
END {
	for (n in deps) {
		count = split(deps[n], list, ",")
		for (i = 1; i <= count; i++) {
			to = dir[name(list[i])]
			if (to != "" && to != from[n]) {
				print rel(from[n]) " " rel(to)
			}
		}
	}
}' "$tmp/packages" | sort -u
