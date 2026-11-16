#!/bin/sh
# Runs the seam specifications' suites against the relay and the disk
# collector built from this checkout (docs/design/seam-specs.md, The harness
# and Defaults and overrides). Run it through make:
#
#   make conformance                                  # every target this host's OS can run
#   make conformance TARGETS="relay-host collector-disk"
#   make conformance SUITES="relay-claim relay-guest-api"
#   make conformance SPECS_DIR=../buildwarden-specs   # suites from a checkout, not the pins
#   OCI_RUNTIME=podman make conformance               # relay-oci with that runtime
#
# A target is a file tools/conformance/targets/<name>.toml. With TARGETS
# unset, every target whose file declares this host's OS runs, and each one
# left out is printed with its reason; a target named in TARGETS that does not
# declare it is an error. With SUITES unset, every pinned specification's
# suite runs against each target; a suite with no case for the target's role
# says so and does not fail.
#
# Each suite's command, bw-conformance-<directory>, is built from its own pin
# module, tools/conformance/<directory>/, with GOWORK=off, so it carries the
# harness its own release names; with SPECS_DIR it is built from that
# checkout's workspace instead. What the targets run is built into
# dist/conformance/: relay (CGO_ENABLED=0, Linux only, for the relay
# targets), collector (collector.exe on Windows), and, for a target whose
# launcher is oci, the image buildwarden-relay:conformance from
# Dockerfile.relay. Every suite runs against every target before the run
# fails; no case is retried. One table is printed for each target.
set -eu

repo=github.com/buildwarden/buildwarden-specs
dirs="bootstrap-config collector-inputs collector-outputs executor-initialize relay-claim relay-guest-api"
targets_dir=tools/conformance/targets
out=dist/conformance
image=buildwarden-relay:conformance

# A go.work above the checkout would give the six pin modules one build
# list, and so one harness for all six; and it would take part in building
# the relay and the collector.
GOWORK=off
export GOWORK

die() {
	echo "conformance: $*" >&2
	exit 1
}

TARGETS="${TARGETS:-}"
SUITES="${SUITES:-}"
SPECS_DIR="${SPECS_DIR:-}"
OCI_RUNTIME="${OCI_RUNTIME:-}"

[ -d "$targets_dir" ] || die "$targets_dir is missing; run from the repository root"
# The checkout's root, as the Go tools take a path: on Windows, Git's sh
# prints /d/... for pwd and D:/... for pwd -W.
root=$(pwd -W 2>/dev/null || pwd)
host=$(go env GOOS)
arch=$(go env GOARCH)
exe=$(go env GOEXE)

# key prints the value of the top-level string key $2 of target file $1, and
# os_of the operating systems its os array names, one line each.
key() {
	sed -n "s/^$2[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$1" | sed -n 1p
}
os_of() {
	sed -n 's/^os[[:space:]]*=[[:space:]]*\[\(.*\)\].*/\1/p' "$1" | tr -d ' "' | tr ',' ' '
}
runs_here() {
	case " $(os_of "$1") " in *" $host "*) return 0 ;; esac
	return 1
}

# The targets: each named one must exist and declare this host's OS; with
# none named, those that declare it, and a line for each one left out.
run_targets=""
if [ -n "$TARGETS" ]; then
	for t in $TARGETS; do
		f="$targets_dir/$t.toml"
		[ -f "$f" ] || die "there is no target $t ($f); the targets are: $(cd "$targets_dir" && ls *.toml | sed 's/\.toml$//' | tr '\n' ' ')"
		runs_here "$f" || die "target $t declares os $(os_of "$f"), and this host's is $host"
		run_targets="$run_targets $t"
	done
else
	for f in "$targets_dir"/*.toml; do
		t=$(basename "$f" .toml)
		if runs_here "$f"; then
			run_targets="$run_targets $t"
		else
			echo "conformance: leaving out $t: it declares os $(os_of "$f"), and this host's is $host"
		fi
	done
fi
[ -n "$run_targets" ] || die "no target in $targets_dir declares os $host"

run_suites=""
if [ -n "$SUITES" ]; then
	for s in $SUITES; do
		case " $dirs " in *" $s "*) ;; *) die "$s is not a specification; the six are: $dirs" ;; esac
		run_suites="$run_suites $s"
	done
else
	run_suites=" $dirs"
fi

if [ -n "$SPECS_DIR" ]; then
	[ -f "$SPECS_DIR/go.work" ] || die "SPECS_DIR=$SPECS_DIR is not a checkout of $repo: it has no go.work"
	specs_work=$(cd "$SPECS_DIR" && { pwd -W 2>/dev/null || pwd; })/go.work
	echo "conformance: SPECS_DIR=$SPECS_DIR: the suites are built from that checkout, not from the pins in tools/conformance/"
fi

mkdir -p "$out/bin" "$out/reports"

# Each suite's command.
for d in $run_suites; do
	bin="$root/$out/bin/bw-conformance-$d$exe"
	pkg=$repo/$d/cmd/bw-conformance-$d
	if [ -n "$SPECS_DIR" ]; then
		(cd "$SPECS_DIR/$d" && GOWORK="$specs_work" go build -buildvcs=false -o "$bin" ./cmd/bw-conformance-$d) ||
			die "bw-conformance-$d does not build from $SPECS_DIR"
	else
		(cd "tools/conformance/$d" && go build -buildvcs=false -o "$bin" "$pkg") ||
			die "bw-conformance-$d does not build from its pin module tools/conformance/$d"
	fi
done

# What the targets run.
need_relay=0
need_collector=0
oci_runtimes=""
for t in $run_targets; do
	f="$targets_dir/$t.toml"
	case "$(key "$f" role)" in
	relay) need_relay=1 ;;
	collector) need_collector=1 ;;
	esac
	if [ "$(key "$f" launcher)" = oci ]; then
		rt=${OCI_RUNTIME:-$(sed -n 's/^runtime[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$f" | sed -n 1p)}
		[ -n "$rt" ] || die "target $t is an oci target that names no runtime"
		case " $oci_runtimes " in *" $rt "*) ;; *) oci_runtimes="$oci_runtimes $rt" ;; esac
	fi
done
if [ "$need_relay" = 1 ]; then
	CGO_ENABLED=0 go build -buildvcs=false -o "$out/relay" ./cmd/relay || die "the relay does not build"
fi
if [ "$need_collector" = 1 ]; then
	go build -buildvcs=false -o "$out/collector$exe" ./cmd/collector || die "the collector does not build"
fi
if [ -n "$oci_runtimes" ]; then
	rm -rf "$out/oci"
	mkdir -p "$out/oci"
	cp "$out/relay" "$out/oci/relay-$arch"
	for rt in $oci_runtimes; do
		command -v "$rt" >/dev/null 2>&1 ||
			die "an oci target names the container runtime $rt, which is not on this host's PATH"
		"$rt" build -f Dockerfile.relay --build-arg "TARGETARCH=$arch" -t "$image" "$out/oci" ||
			die "$rt cannot build the image $image from Dockerfile.relay"
	done
fi

# The runs. A target whose launcher is oci is given OCI_RUNTIME, when it is
# set, through a copy of its file with that runtime in place of its own.
failed=0
for t in $run_targets; do
	f="$targets_dir/$t.toml"
	if [ -n "$OCI_RUNTIME" ] && [ "$(key "$f" launcher)" = oci ]; then
		mkdir -p "$out/targets"
		sed "s/^runtime[[:space:]]*=.*/runtime = \"$OCI_RUNTIME\"/" "$f" >"$out/targets/$t.toml"
		f="$out/targets/$t.toml"
		echo "conformance: $t runs with OCI_RUNTIME=$OCI_RUNTIME, through $f"
	fi
	mkdir -p "$out/reports/$t"
	table="$out/reports/$t/table"
	: >"$table"
	for d in $run_suites; do
		report="$out/reports/$t/$d.txt"
		echo "== $t: $d"
		status=0
		"$out/bin/bw-conformance-$d$exe" run -target "$f" >"$report" 2>&1 || status=$?
		cat "$report"
		counts=$(sed -n 's/^.* \(v[^ ,]*\), conformance \([^ ,]*\), against .*: \([0-9]*\) passed, \([0-9]*\) failed, \([0-9]*\) not applicable$/\1 \2 \3 \4 \5/p' "$report" | tail -n 1)
		if [ "$status" != 0 ]; then
			failed=1
			[ -n "$counts" ] || counts="- - - - -"
			echo "$d $counts exit-$status" >>"$table"
		elif [ -z "$counts" ]; then
			failed=1
			echo "$d - - - - - no-report" >>"$table"
		else
			echo "$d $counts ok" >>"$table"
		fi
	done
	echo
	echo "== $t ($f)"
	{
		echo "specification version harness passed failed n/a result"
		cat "$table"
	} | awk '{ printf "%-20s %-8s %-8s %6s %6s %4s  %s\n", $1, $2, $3, $4, $5, $6, $7 }'
	echo
done
if [ "$failed" != 0 ]; then
	die "a suite failed or did not run (the tables above; each report is in $out/reports/)"
fi
echo "conformance: every suite passed against every target that ran:$run_targets"
