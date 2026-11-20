#!/usr/bin/env bash
# Scheduled native session for one machine (FLOW-09), macOS or Linux.
#
#   tools/workflow/native/native-tick.sh <machine>      # e.g. macbook
#
# Run by launchd (install-macos.sh) from a dedicated checkout of
# buildwarden/buildwarden that nobody edits by hand. It syncs the checkout,
# each clone of a second repository the machine lists in hosts.yaml
# (.dev/checkouts/<owner>/<repo>, reset to its trunk), and the workflow
# branch, asks `wf gate` whether the machine has native work, and only then
# starts a headless Claude Code session on the workflow-native skill
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
# The session's worker is host:<name>/<machine>.<run>, and its claims hold a
# 20-hour lease it renews while it works (FLOW-04). Before the gate the
# wrapper releases every lease an earlier scheduled session of this machine
# left, since the gate does not offer a leased step; when a session it
# started ends, it releases every lease that session still holds. Before the
# gate it also removes each .scratch/ worktree whose item is done or whose
# branch is gone from origin, and its local branch, with git's safe forms
# only: a worktree with changes and a branch git branch -d refuses are kept
# and named in the status reason and the session's prompt (FLOW-16).
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

outcome="" reason="" stage="setup" session_started="" leftover="" dirty_worktree=""
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

# fetch runs git fetch, retrying a network or auth failure with backoff
# (2s, 4s, 8s): GitHub refuses a valid key now and then for a few seconds.
fetch() {
	local delay
	for delay in 2 4 8 0; do
		git fetch -q "$@" && return
		[ "$delay" -eq 0 ] && return 1
		echo "fetch $* failed; retrying in ${delay}s"
		sleep "$delay"
	done
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
	fetch origin workflow || { echo "status: cannot fetch workflow"; return; }
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
		fetch origin workflow
	done
	echo "status: not pushed after 3 attempts"
}

# release_leases runs dist/wf release with the arguments given on an
# up-to-date workflow checkout and pushes what it released with wf commit.
# A checkout with a session's leftovers is not reset; record_status saves them.
release_leases() {
	local wt=.dev/workflow out paths
	cd "$repo" || return 1
	if [ -z "$(git -C "$wt" status --porcelain)" ] &&
		[ -z "$(git -C "$wt" rev-list origin/workflow..HEAD 2>/dev/null)" ] &&
		fetch origin workflow; then
		git -C "$wt" reset -q --hard origin/workflow
	fi
	out=$(dist/wf release -state "$wt" "$@") || return 1
	[ -n "$out" ] || return 0
	printf '%s\n' "$out"
	paths=$(printf '%s\n' "$out" | awk -F '\t' '$1 == "released" { print "queue/" $2 ".yaml" }' |
		sort -u)
	# shellcheck disable=SC2086 # one queue file per line, names without spaces
	dist/wf commit -state "$wt" -worker "native/$machine.$run" \
		-m "$machine: release leases ($machine.$run)" $paths
}

# note_leftover adds one note to leftover, the cleanup's "; "-separated list.
note_leftover() { leftover="${leftover:+$leftover; }$1"; }

# clean_worktrees removes each worktree under .scratch/ on a branch
# feat/<item> whose item is done or whose branch is gone from origin after the
# prune fetch, and then the local branch, with git's safe forms only
# (FLOW-16). With no argument it cleans the dedicated checkout's worktrees,
# .scratch/<name>; with <owner>/<repo>, those of that repository's clone,
# .scratch/<owner>/<repo>/<name>, each note naming the repository.
clean_worktrees() {
	local d name branch item q clone=. scratch=.scratch/ in=""
	if [ -n "${1:-}" ]; then
		clone=.dev/checkouts/$1 scratch=.scratch/$1/ in=" in $1"
	fi
	for d in "$scratch"*/; do
		[ -d "$d" ] || continue
		name=${d%/}
		[ "$(git -C "$d" rev-parse --show-prefix 2>/dev/null)" = "" ] || continue
		[ -f "$d.git" ] || continue
		branch=$(git -C "$d" symbolic-ref -q --short HEAD) || continue
		item=${branch#feat/}
		[ "$item" != "$branch" ] || continue
		q=.dev/workflow/queue/$item.yaml
		if ! grep -qx 'status: done' "$q" 2>/dev/null &&
			git -C "$clone" rev-parse -q --verify "refs/remotes/origin/$branch" >/dev/null; then
			continue
		fi
		if [ -n "$(git -C "$d" status --porcelain)" ]; then
			note_leftover "leftover worktree $name has changes"
			dirty_worktree=1
			continue
		fi
		git -C "$clone" worktree remove "$repo/$name" || {
			note_leftover "leftover worktree $name not removed"
			continue
		}
		echo "cleanup: removed $name ($branch$in)"
		git -C "$clone" branch -d "$branch" ||
			note_leftover "branch $branch$in not deleted (not merged; FLOW-16)"
	done
}

# reset_clones resets each clone of a second repository the machine lists,
# .dev/checkouts/<owner>/<repo>, to its trunk (the one its config.yaml entry
# gives), whatever branch or changes a session left there; with cleanup it
# also cleans the clone's worktrees. A clone that is not there stops the
# run: its owner makes it (LOOP-05).
reset_clones() {
	local list name trunk c tab
	tab=$(printf '\t')
	list=$(dist/wf native -state .dev/workflow -repo . -machine "$machine" -clones) ||
		stop "wf native -clones failed"
	while IFS="$tab" read -r name trunk; do
		[ -n "$name" ] || continue
		c=.dev/checkouts/$name
		git -C "$c" rev-parse --git-dir >/dev/null 2>&1 ||
			stop "no clone of $name at $c (LOOP-05: its owner clones it)"
		(cd "$c" && fetch --prune origin) || stop "fetch of $name failed"
		git -C "$c" switch -q -f -C "$trunk" "origin/$trunk"
		git -C "$c" reset -q --hard "origin/$trunk"
		git -C "$c" clean -fdq
		git -C "$c" worktree prune
		[ "${1:-}" != cleanup ] || clean_worktrees "$name"
	done <<LIST
$list
LIST
}

# with_leftover appends the cleanup's notes to reason, and records a dirty
# worktree as a failure whatever the run's outcome was.
with_leftover() {
	[ -n "$leftover" ] || return 0
	reason="$reason; $leftover"
	[ -z "$dirty_worktree" ] || outcome=failed
}

# shellcheck disable=SC2329 # called from the EXIT trap
on_exit() {
	local rc=$?
	set +e
	if [ -z "$reason" ]; then
		outcome=failed reason="exit $rc during $stage"
	fi
	if [ -x "$repo/dist/wf" ] && [ -n "$session_started" ]; then
		release_leases -session "$machine.$run" -why "session ended (exit $rc)" ||
			echo "release: the session's leases were not released"
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
fetch --prune origin
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
stage="worktrees"
clean_worktrees
[ -z "$leftover" ] || echo "cleanup: $leftover"
stage="flow18-check"
tools/workflow/flow18-check.sh
# A session that cannot be recorded takes no led step (FLOW-18).
recorded=""
entire_status=$(entire status 2>&1 || true)
case $entire_status in
*buildwarden/buildwarden-sessions*) ;;
*)
	recorded=" Sessions not recorded (FLOW-18): entire status does not name buildwarden/buildwarden-sessions; take no led step this run, and put that line first in your digest file."
	echo "flow18: entire status does not name the private sessions remote"
	;;
esac
stage="build wf"
mkdir -p dist
(cd tools/workflow && go build -o ../../dist/wf ./cmd/wf)
stage="clones"
reset_clones cleanup
[ -z "$leftover" ] || echo "cleanup: $leftover"
# The gate does not offer a step leased to another worker, so the leases an
# earlier scheduled session of this machine left are released first.
stage="release"
release_leases -machine "$machine" -stale -current "$machine.$run" \
	-why "an earlier session on $machine left it" || stop "wf release of stale leases failed"
stage="gate"

stamp="$state/gate-$machine.json"
if [ "${BW_FORCE:-}" = 1 ]; then
	echo "gate: skipped (BW_FORCE=1)"
else
	set +e
	gate_out=$(dist/wf gate -state .dev/workflow -repo . -machine "$machine" -stamp "$stamp" \
		-sweep-days "${BW_SWEEP_DAYS:-7}")
	rc=$?
	set -e
	printf '%s\n' "$gate_out"
	case $rc in
	0) ;;
	3)
		outcome=idle
		reason=$(printf '%s\n' "$gate_out" | awk -F '\t' '$1 == "idle" { print "idle: " $2; exit }')
		[ -n "$reason" ] || reason="idle: gate found no work"
		[ -z "$recorded" ] || reason="$reason; sessions not recorded (FLOW-18)"
		with_leftover
		echo "idle: no agent started"
		exit 0
		;;
	*) stop "wf gate failed" ;;
	esac
fi

stage="native assets"
# A host's cached build assets follow the trunk, so a session never tests
# behind a merged fix: each listed repository's profile file names, as
# native.prepare, the commands for the hosts it has assets for (here
# windows-hyperv: the relay boot VHDX in warden's cache). A failed prepare
# stales only the hosts it is listed under: the session still runs, told to
# skip them, so the machine's other hosts are not blocked.
prepared=$(dist/wf native -state .dev/workflow -repo . -machine "$machine" -prepare) ||
	stop "wf native -prepare failed"
printf '%s\n' "$prepared"
stale=$(printf '%s\n' "$prepared" | awk -F '\t' '$1 == "stale" { printf "%s%s", sep, $2; sep = " " }')
[ -z "$stale" ] || echo "native assets: native.prepare failed; skipping $stale this run"
skip=""
[ -z "$stale" ] || skip=" Stale hosts: $stale (native.prepare failed); skip their steps and gaps this run and say why in the digest."
leftovers=""
[ -z "$leftover" ] || leftovers=" Leftovers: $leftover."

command -v claude >/dev/null || stop "claude is not on PATH"
stage="claude"
# Stamp what this session starts from, so a branch that moves while it runs
# triggers the next one.
dist/wf gate -state .dev/workflow -repo . -machine "$machine" -stamp "$stamp.pending" -record
set +e
session_started=1
claude -p "Invoke the workflow-native skill with the arguments \"unattended $machine\" and follow it exactly. Worker session id: $machine.$run.$skip$leftovers$recorded" \
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
[ -z "$stale" ] || reason="$reason; skipped $stale (native.prepare failed)"
[ -z "$recorded" ] || reason="$reason; sessions not recorded (FLOW-18)"
with_leftover
stage="cleanup"

# Leave the checkout on main and each clone on its trunk, clean, whatever
# the session did.
git switch -q -f main && git reset -q --hard origin/main
reset_clones

if [ $rc -eq 0 ]; then
	mv "$stamp.pending" "$stamp"
fi
exit $rc
