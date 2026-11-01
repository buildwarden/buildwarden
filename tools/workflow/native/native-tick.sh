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
# Every exit after the lock is taken records the run in
# native/status/<machine>.yaml on the workflow branch (FLOW-09): outcome
# worked, idle, skipped or failed, with a one-line reason. It is pushed only
# when the outcome or reason changes, or the last push is a day old, so idle
# nights do not churn the branch. Workflow changes a session left uncommitted
# or unpushed are saved under $BW_STATE_DIR/leftover/<run>/ and reported as a
# failure, never reset away.
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

outcome="" reason="" stage="setup"
# skip and stop end the run with a fixed one-line reason, so an unchanged
# situation writes an unchanged status file.
skip() { outcome=skipped reason="skip: $1"; echo "$reason"; exit 0; }
stop() { outcome=failed reason="stop: $1"; echo "$reason"; exit 1; }

# epoch converts a UTC timestamp to seconds, with GNU date (Git Bash, Linux)
# or BSD date (macOS).
# shellcheck disable=SC2329 # called from the EXIT trap
epoch() {
	date -u -d "$1" +%s 2>/dev/null || date -u -j -f %Y-%m-%dT%H:%M:%SZ "$1" +%s 2>/dev/null ||
		echo 0
}

# save_leftover keeps what a session left in the workflow checkout before it
# is reset: uncommitted changes, untracked files and unpushed commits.
# shellcheck disable=SC2329 # called from the EXIT trap
save_leftover() {
	local wt="$1" d="$state/leftover/$run"
	mkdir -p "$d"
	git -C "$wt" diff HEAD >"$d/uncommitted.patch"
	git -C "$wt" ls-files -z --others --exclude-standard |
		tar -C "$wt" --null -T - -cf "$d/untracked.tar"
	git -C "$wt" format-patch -q -o "$d/commits" origin/workflow..HEAD
	echo "saved the session's leftover workflow changes in $d"
}

# reset_workflow returns 1, after saving them, when a session left changes or
# unpushed commits in the workflow checkout.
reset_workflow() {
	local wt="$1"
	if [ -n "$(git -C "$wt" status --porcelain)" ] ||
		[ -n "$(git -C "$wt" rev-list origin/workflow..HEAD 2>/dev/null)" ]; then
		save_leftover "$wt"
		return 1
	fi
}

# record_status writes and pushes native/status/<machine>.yaml (see the top).
# shellcheck disable=SC2329 # called from the EXIT trap
record_status() {
	local wt=.dev/workflow f prev_outcome prev_reason prev_since prev_last since attempt
	cd "$repo" || return
	git fetch -q origin workflow || { echo "status: cannot fetch workflow"; return; }
	if [ ! -d "$wt" ]; then
		git worktree add -q -B workflow "$wt" origin/workflow || return
	fi
	if ! reset_workflow "$wt"; then
		outcome=failed reason="session left uncommitted workflow changes"
	fi
	f="$wt/native/status/$machine.yaml"
	for attempt in 1 2 3; do
		git -C "$wt" reset -q --hard origin/workflow
		git -C "$wt" clean -fdq
		prev_outcome=$(sed -n 's/^outcome: //p' "$f" 2>/dev/null)
		prev_reason=$(sed -n 's/^reason: "\(.*\)"$/\1/p' "$f" 2>/dev/null)
		prev_since=$(sed -n 's/^since: //p' "$f" 2>/dev/null)
		prev_last=$(sed -n 's/^last_run: //p' "$f" 2>/dev/null)
		if [ "$prev_outcome" = "$outcome" ] && [ "$prev_reason" = "$reason" ] &&
			[ $(($(date -u +%s) - $(epoch "$prev_last"))) -lt 86400 ]; then
			echo "status: $outcome, unchanged since $prev_since; not pushed"
			return
		fi
		since="$now"
		[ "$prev_outcome" = "$outcome" ] && [ -n "$prev_since" ] && since="$prev_since"
		mkdir -p "$(dirname "$f")"
		cat >"$f" <<-EOF
			# Written by native-tick.sh; see the workflow README. One line of reason, no logs (FLOW-18).
			machine: $machine
			last_run: $now
			outcome: $outcome
			reason: "$reason"
			since: $since
		EOF
		git -C "$wt" add "native/status/$machine.yaml"
		git -C "$wt" commit -q -m "$machine: status $outcome"
		if git -C "$wt" push -q origin workflow; then
			echo "status: pushed $outcome ($reason)"
			return
		fi
		echo "status: push rejected (attempt $attempt); refetching"
		git fetch -q origin workflow
	done
	echo "status: not pushed after 3 attempts"
}

# shellcheck disable=SC2329 # called from the EXIT trap
on_exit() {
	local rc=$?
	set +e
	if [ -z "$reason" ]; then
		outcome=failed reason="exit $rc during $stage"
	fi
	reason=$(printf '%s' "$reason" | tr -d '"\134' | tr '\n' ' ' | cut -c1-120)
	record_status
	rm -rf "$lock"
	exit "$rc"
}
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
trap on_exit EXIT

# Keep a month of logs.
find "$state/logs" -name '*.log' -mtime +30 -delete 2>/dev/null || true

if [ "${BW_FORCE:-}" != 1 ] && [ "$(uname -s)" = Darwin ] &&
	! pmset -g batt | grep -q "AC Power"; then
	skip "on battery"
fi
free_gb=$(df -Pk "$repo" | awk 'NR==2 {print int($4/1024/1024)}')
if [ "$free_gb" -lt "${BW_MIN_FREE_GB:-40}" ]; then
	echo "${free_gb}G free, need ${BW_MIN_FREE_GB:-40}G (VM images and test caches)"
	skip "low disk"
fi

cd "$repo"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
	git status --short
	stop "the dedicated checkout has local changes"
fi
stage="sync"
git fetch -q --prune origin
git switch -q main
git reset -q --hard origin/main
git worktree prune
if [ -d .dev/workflow ]; then
	reset_workflow .dev/workflow || true
	git -C .dev/workflow reset -q --hard origin/workflow
	git -C .dev/workflow clean -fdq
else
	git worktree add -q -B workflow .dev/workflow origin/workflow
fi
stage="flow18-check"
tools/workflow/flow18-check.sh
stage="build wf"
mkdir -p dist
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
stage="gate"

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
	3) outcome=idle reason="idle: gate found no work"; echo "idle: no agent started"; exit 0 ;;
	*) stop "wf gate failed" ;;
	esac
fi

stage="native assets"
# A host's cached build assets follow origin/main, so a session never tests
# behind a merged fix. windows-hyperv: the relay boot VHDX in warden's cache.
# A failed refresh stales only the hosts that use the asset: the session still
# runs, told to skip them, so the machine's other hosts are not blocked.
served=$(awk -v m="$machine" '
	$1 == "-" && $2 == "name:" { mine = ($3 == m); next }
	mine && $1 == "hosts:" { print; mine = 0 }' .dev/workflow/hosts.yaml)
stale=""
case "$served" in
*windows-hyperv*)
	if ! tools/relay-vm/hyperv/refresh-cache.sh; then
		stale="windows-hyperv"
		echo "native assets: relay boot VHDX refresh failed; skipping windows-hyperv this run"
	fi
	;;
esac
skip=""
[ -z "$stale" ] || skip=" Stale hosts: $stale (relay boot VHDX refresh failed); skip their steps and gaps this run and say why in the digest."

command -v claude >/dev/null || stop "claude is not on PATH"
stage="claude"
# Stamp what this session starts from, so a branch that moves while it runs
# triggers the next one.
dist/wf gate -state .dev/workflow -repo . -machine "$machine" -stamp "$stamp.pending" -record
set +e
claude -p "Invoke the workflow-native skill with the arguments \"unattended $machine\" and follow it exactly. Worker session id: $run.$skip" \
	--settings tools/workflow/native/settings.native.json \
	--permission-mode acceptEdits \
	--max-turns "${BW_MAX_TURNS:-400}"
rc=$?
set -e
echo "claude exited $rc"
if [ $rc -eq 0 ]; then
	outcome=worked
else
	outcome=failed
fi
reason="claude exited $rc"
[ -z "$stale" ] || reason="$reason; skipped $stale (relay boot VHDX refresh failed)"
stage="cleanup"

# Leave the checkout on main and clean, whatever the session did.
git switch -q -f main && git reset -q --hard origin/main

if [ $rc -eq 0 ]; then
	mv "$stamp.pending" "$stamp"
fi
exit $rc
