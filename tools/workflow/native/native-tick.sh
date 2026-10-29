#!/usr/bin/env bash
# Scheduled native session for one machine (FLOW-09), macOS or Linux.
#
#   tools/workflow/native/native-tick.sh <machine>      # e.g. macbook
#
# Run by launchd (install-macos.sh) from a dedicated checkout of
# buildwarden/buildwarden that nobody edits by hand. It syncs the checkout and
# the workflow branch, asks `wf gate` whether the machine has native work, and
# only then starts a headless Claude Code session on the workflow-native skill
# in unattended mode. An idle night starts no agent and costs nothing.
#
# Environment (all optional):
#   BW_MIN_FREE_GB  free disk needed to start (default 40)
#   BW_SWEEP_DAYS   re-sweep open backlog gaps after this many days (default 7)
#   BW_MAX_TURNS    turn cap for the agent session (default 400)
#   BW_STATE_DIR    stamp, lock and logs (default ~/Library/Application Support/buildwarden-native
#                   on macOS, ~/.local/state/buildwarden-native elsewhere)
#   BW_FORCE=1      skip the gate and the AC-power check (a manual run)
set -euo pipefail

machine="${1:?usage: native-tick.sh <machine>}"
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
if [ "$(uname -s)" = Darwin ]; then
	state="${BW_STATE_DIR:-$HOME/Library/Application Support/buildwarden-native}"
else
	state="${BW_STATE_DIR:-$HOME/.local/state/buildwarden-native}"
fi
mkdir -p "$state/logs"
run="$(date -u +%Y%m%dT%H%M%SZ)"
log="$state/logs/$run.log"
exec >>"$log" 2>&1
echo "native-tick $machine $run in $repo"

# launchd starts with a bare PATH; pick up Homebrew, mise and ~/.local/bin.
export PATH="$HOME/.local/bin:$HOME/.local/share/mise/shims:/opt/homebrew/bin:/usr/local/bin:$PATH"

# One session per machine at a time; a stale lock (no live pid) is taken over.
lock="$state/lock"
if ! mkdir "$lock" 2>/dev/null; then
	if kill -0 "$(cat "$lock/pid" 2>/dev/null)" 2>/dev/null; then
		echo "skip: another native session holds $lock"
		exit 0
	fi
	echo "taking over a stale lock"
fi
echo $$ >"$lock/pid"
trap 'rm -rf "$lock"' EXIT

# Keep a month of logs.
find "$state/logs" -name '*.log' -mtime +30 -delete 2>/dev/null || true

if [ "${BW_FORCE:-}" != 1 ] && [ "$(uname -s)" = Darwin ] &&
	! pmset -g batt | grep -q "AC Power"; then
	echo "skip: on battery"
	exit 0
fi
free_gb=$(df -Pk "$repo" | awk 'NR==2 {print int($4/1024/1024)}')
if [ "$free_gb" -lt "${BW_MIN_FREE_GB:-40}" ]; then
	echo "skip: ${free_gb}G free, need ${BW_MIN_FREE_GB:-40}G (VM images and test caches)"
	exit 0
fi

cd "$repo"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
	echo "stop: the dedicated checkout has local changes; it is for the native loop only"
	git status --short
	exit 1
fi
git fetch -q --prune origin
git switch -q main
git reset -q --hard origin/main
git worktree prune
if [ -d .dev/workflow ]; then
	git -C .dev/workflow reset -q --hard origin/workflow
else
	git worktree add -q -B workflow .dev/workflow origin/workflow
fi
tools/workflow/flow18-check.sh
mkdir -p dist
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)

stamp="$state/gate-$machine.json"
if [ "${BW_FORCE:-}" = 1 ]; then
	echo "gate: skipped (BW_FORCE=1)"
else
	set +e
	dist/wf gate -state .dev/workflow -repo . -machine "$machine" -stamp "$stamp" \
		-sweep-days "${BW_SWEEP_DAYS:-7}"
	rc=$?
	set -e
	case $rc in
	0) ;;
	3) echo "idle: no agent started"; exit 0 ;;
	*) echo "stop: wf gate failed ($rc)"; exit 1 ;;
	esac
fi

command -v claude >/dev/null || { echo "stop: claude is not on PATH"; exit 1; }
# Stamp what this session starts from, so a branch that moves while it runs
# triggers the next one.
dist/wf gate -state .dev/workflow -repo . -machine "$machine" -stamp "$stamp.pending" -record
set +e
claude -p "Invoke the workflow-native skill with the arguments \"unattended $machine\" and follow it exactly. Worker session id: $run." \
	--settings tools/workflow/native/settings.native.json \
	--permission-mode acceptEdits \
	--max-turns "${BW_MAX_TURNS:-400}"
rc=$?
set -e
echo "claude exited $rc"

# Leave the checkout on main and clean, whatever the session did.
git switch -q -f main && git reset -q --hard origin/main

if [ $rc -eq 0 ]; then
	mv "$stamp.pending" "$stamp"
fi
exit $rc
