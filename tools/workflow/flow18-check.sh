#!/bin/sh
# FLOW-18: agent sessions are recorded only to the private sessions repository.
# Fails when .entire/settings.json does not name it as checkpoint_remote, or when
# the public remote carries any entire/ ref (checkpoint refs or shadow branches).
#
#   tools/workflow/flow18-check.sh [remote]   # remote defaults to origin
set -eu

remote="${1:-origin}"
want="buildwarden/buildwarden-sessions"
settings=".entire/settings.json"
fail=0

if [ ! -f "$settings" ]; then
	echo "FLOW-18: $settings is missing" >&2
	fail=1
else
	got=$(jq -r '.strategy_options.checkpoint_remote | "\(.provider):\(.repo)"' "$settings")
	if [ "$got" != "github:$want" ]; then
		echo "FLOW-18: $settings checkpoint_remote is '$got', want 'github:$want'" >&2
		fail=1
	fi
fi

# Any ref with an entire/ path component: refs/entire/checkpoints/..., and the
# entire/... branches and tags older CLIs pushed.
refs=$(git ls-remote "$remote") || { echo "FLOW-18: cannot list $remote" >&2; exit 1; }
leaked=$(printf '%s\n' "$refs" | awk '{print $2}' | grep -E '^refs/(entire/|heads/entire/|tags/entire/)' || true)
if [ -n "$leaked" ]; then
	echo "FLOW-18: $remote carries entire/ refs; delete them (the human's call, FLOW-16):" >&2
	printf '%s\n' "$leaked" | sed 's/^/  /' >&2
	fail=1
fi

[ "$fail" -eq 0 ] && echo "FLOW-18: ok ($settings names $want; no entire/ refs on $remote)"
exit "$fail"
