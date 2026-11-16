#!/bin/sh
# Moves the pins of the seam specifications (Decision 7 of
# docs/design/seam-specs.md): each tools/conformance/<directory>/go.mod
# requires one specification of buildwarden/buildwarden-specs at one version
# and names its command in a tool directive. Run it through make:
#
#   make pin-specs SPECS="relay-claim@v0.2.0 collector-outputs@v0.1.1"
#   make pin-specs SPECS=latest     # all six at their newest releases
#   make pin-specs REF=<commit>     # all six at one commit, on a branch only
#
# It is given SPECS or REF, never both and never neither: it has no default.
#
# A module it moves is written from nothing (the module line, the go line of
# the pinned specification's own go.mod, the tool directive and the one
# requirement) and then tidied, because go get in place keeps the harness
# version of the old pin. With REF, a specification's go.mod may name a
# harness version that has no tag yet; the module then gets one replace line
# pointing the harness at the same commit. Whether the version has its tag is
# asked before the module is written, not found out from a failed build.
#
# Every module is written and its command built in a scratch directory first,
# and only when all of them build are the files moved into place and the refs
# on docs/book/src/concepts/seams.md rewritten, so a failure changes nothing.
set -eu

repo=github.com/buildwarden/buildwarden-specs
harness=$repo/conformance
kit=github.com/buildwarden/buildwarden/tools/conformance
dirs="bootstrap-config collector-inputs collector-outputs executor-initialize relay-claim relay-guest-api"
root=tools/conformance
page=docs/book/src/concepts/seams.md
release_re='^v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$'
pseudo_re='[0-9]\{14\}-[0-9a-f]\{12\}$'

# A go.work above the checkout would give the six modules one build list,
# and so one harness for all six.
GOWORK=off
export GOWORK

die() {
	echo "pin-specs: $*" >&2
	exit 1
}

SPECS="${SPECS:-}"
REF="${REF:-}"
if [ -z "$SPECS" ] && [ -z "$REF" ]; then
	die 'give SPECS="<directory>@<version> ...", SPECS=latest or REF=<commit>'
fi
if [ -n "$SPECS" ] && [ -n "$REF" ]; then
	die "give SPECS or REF, not both"
fi
[ -f "$page" ] || die "$page is missing; run from the repository root"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

is_dir() {
	case " $dirs " in *" $1 "*) return 0 ;; esac
	return 1
}

# require_version prints the version a go.mod file requires module $2 at, in
# either the one-line or the block form, and nothing when it does not.
require_version() {
	awk -v m="$2" '
		/^require[ \t]*\($/ { blk = 1; next }
		blk && /^\)/ { blk = 0; next }
		blk && $1 == m { print $2 }
		!blk && $1 == "require" && $2 == m { print $3 }
	' "$1"
}

# resolve prints the version a module query resolves to, then the path of
# that version's go.mod, one to a line. It runs in the scratch directory so
# that no module's build list takes part.
resolve() {
	(cd "$tmp" && go list -m -f '{{.Version}}{{"\n"}}{{.GoMod}}' "$1@$2") ||
		die "cannot resolve $1@$2"
}

# harness_tagged reports whether the harness version $1 has its tag. Only
# Go's "unknown revision" means it has none; any other failure stops.
harness_tagged() {
	if out=$(cd "$tmp" && go list -m "$harness@$1" 2>&1); then
		return 0
	fi
	case "$out" in *"unknown revision"*) return 1 ;; esac
	die "cannot ask whether $harness@$1 has its tag: $out"
}

# ref_of prints the ref the seams page links directory $1 at, for the version
# $2 its module pins: the tag <directory>/<version> for a release, and for a
# pseudo-version the twelve-digit commit it names. specs/specs_test.go
# derives it the same way.
ref_of() {
	if echo "$2" | grep -q "$pseudo_re"; then
		echo "$2" | sed 's/.*-//'
	else
		echo "$1/$2"
	fi
}

# What to move: one "<directory> <query>" line for each module.
plan="$tmp/plan"
: >"$plan"
if [ -n "$REF" ]; then
	echo "$REF" | grep -q '^[0-9a-f]\{7,40\}$' ||
		die "REF=$REF is not a commit; give its hexadecimal hash"
	for d in $dirs; do echo "$d $REF" >>"$plan"; done
elif [ "$SPECS" = latest ]; then
	for d in $dirs; do echo "$d latest" >>"$plan"; done
else
	for s in $SPECS; do
		d=${s%%@*}
		v=${s#*@}
		[ "$d@$v" = "$s" ] || die "$s is not <directory>@<version>"
		is_dir "$d" || die "$d is not a specification; the six are: $dirs"
		echo "$v" | grep -q "$release_re" ||
			die "$s: $v is not a release version vX.Y.Z"
		! grep -q "^$d " "$plan" || die "$d is named twice"
		echo "$d $v" >>"$plan"
	done
	# A module that is left as it is must already be on a release: after
	# REF, the move back names all six or is SPECS=latest.
	for d in $dirs; do
		grep -q "^$d " "$plan" && continue
		mod="$root/$d/go.mod"
		[ -f "$mod" ] || die "$mod is missing; name $d in SPECS too"
		v=$(require_version "$mod" "$repo/$d")
		if ! echo "$v" | grep -q "$release_re" || grep -q '^replace' "$mod"; then
			die "$d is pinned at $v, not a release; name it in SPECS too, or give SPECS=latest"
		fi
	done
fi

while read -r d q <&3; do
	grep -q "\`$d@[^\`]*\`" "$page" || die "$page has no \`$d@<version>\` cell"
done 3<"$plan"

# Ask and write each module in the scratch directory.
while read -r d q <&3; do
	mod=$repo/$d
	out=$(resolve "$mod" "$q")
	v=$(echo "$out" | sed -n 1p)
	gomod=$(echo "$out" | sed -n 2p)
	if [ -z "$REF" ] && ! echo "$v" | grep -q "$release_re"; then
		die "$mod@$q is $v, not a release"
	fi
	goline=$(sed -n 's/^go[[:space:]][[:space:]]*\([^[:space:]]*\).*/\1/p' "$gomod")
	[ -n "$goline" ] || die "$mod@$v names no go version"
	hv=$(require_version "$gomod" "$harness")
	[ -n "$hv" ] || die "$mod@$v requires no harness ($harness)"

	w="$tmp/$d"
	mkdir -p "$w"
	{
		echo "module $kit/$d"
		echo
		echo "go $goline"
		echo
		echo "tool $mod/cmd/bw-conformance-$d"
		echo
		echo "require $mod $v"
	} >"$w/go.mod"
	if ! harness_tagged "$hv"; then
		[ -n "$REF" ] ||
			die "$mod@$v names harness $hv, which has no tag; a release names a released harness"
		hpin=$(resolve "$harness" "$REF" | sed -n 1p)
		(cd "$w" && go mod edit -replace="$harness@$hv=$harness@$hpin")
	fi
	echo "pin-specs: $d at $v (harness $hv, go $goline)"
	(cd "$w" && go mod tidy) || die "go mod tidy failed for $d"
	(cd "$w" && go tool "bw-conformance-$d" version) ||
		die "bw-conformance-$d does not build at $v"
	echo "$(ref_of "$d" "$v") $v" >"$w/ref"
done 3<"$plan"

# Every module builds: move them into place and rewrite the page.
cp "$page" "$tmp/page"
while read -r d q <&3; do
	w="$tmp/$d"
	read -r ref v <"$w/ref"
	sed -E \
		-e "s#(buildwarden-specs/(blob|tree)/)[^)]*/($d/[^)/]*\))#\1$ref/\3#g" \
		-e "s#\`$d@[^\`]*\`#\`$d@$v\`#g" \
		"$tmp/page" >"$tmp/page.new"
	mv "$tmp/page.new" "$tmp/page"
done 3<"$plan"
while read -r d q <&3; do
	mkdir -p "$root/$d"
	cp "$tmp/$d/go.mod" "$tmp/$d/go.sum" "$root/$d/"
done 3<"$plan"
cp "$tmp/page" "$page"
