#!/usr/bin/env bash
# FLOW-14: an enforcement change is checked by main's enforcement, not its own.
# When the branch changes tools/tier1/ or tools/workflow/check/, this builds
# tier1 from the merge base with BASE, runs it and the branch's tier1 over the
# same diff, and fails when the branch's report drops a finding on a rule that
# no approved rule_changes entry names (wf drops). The comparison itself is
# built from the merge base once main has it.
#
#   tools/workflow/enforcement-check.sh [BASE [tier1 flags...]]
#
# BASE defaults to origin/main. Run by `make enforcement` at Review and by the
# CI tier1 job.
set -euo pipefail

base_ref="${1:-origin/main}"
[ $# -gt 0 ] && shift
cd "$(git rev-parse --show-toplevel)"
root="$PWD"
base="$(git merge-base "$base_ref" HEAD)"

if [ -z "$(git diff --name-only "$base" HEAD -- tools/tier1 tools/workflow/check)" ]; then
	echo "enforcement-check: the branch changes no enforcement (FLOW-14)"
	exit 0
fi

tmp="$(mktemp -d)"
cleanup() {
	git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT
git worktree add --detach -q "$tmp/base" "$base"
exe="$(go env GOEXE)"

(cd "$tmp/base/tools/tier1" && go build -o "$tmp/tier1-main$exe" ./cmd/tier1)
(cd tools/tier1 && go build -o "$tmp/tier1-branch$exe" ./cmd/tier1)

# tier1 exits 1 on a MUST finding, which is a report to compare, not an error.
tier1() {
	local bin="$1" out="$2" code=0
	shift 2
	"$bin" -base "$base" -json "$@" >"$out" || code=$?
	if [ "$code" -gt 1 ]; then
		echo "enforcement-check: $(basename "$bin") exited $code" >&2
		exit 2
	fi
}
tier1 "$tmp/tier1-main$exe" "$tmp/main.json" "$@"
tier1 "$tmp/tier1-branch$exe" "$tmp/branch.json" "$@"

src=tools/workflow
if grep -qs '^func drops(' "$tmp/base/tools/workflow/cmd/wf/main.go"; then
	src="$tmp/base/tools/workflow"
fi
(cd "$src" && go build -o "$tmp/wf$exe" ./cmd/wf)
"$tmp/wf$exe" drops -repo "$root" -base "$base" -main "$tmp/main.json" \
	-branch "$tmp/branch.json"
