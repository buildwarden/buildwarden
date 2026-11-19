#!/bin/sh
# native-push.sh: the one route by which an unattended native session pushes a
# feature branch (FLOW-17, workflow-native skill).
#
#   tools/workflow/native/native-push.sh <name>
#   tools/workflow/native/native-push.sh <name> <owner>/<repo>
#
# Run from the checkout root. It pushes the branch feat/<item> checked out in
# the worktree .scratch/<name> to the same name on origin, one refspec and no
# "+", so git refuses a non-fast-forward and nothing else moves; a first push
# sets the upstream. It refuses a branch whose item is not in progress in the
# queue state at .dev/workflow, a detached head, any other branch name, and
# any argument after the name.
#
# With <owner>/<repo>, a second repository the item works in, it pushes the
# worktree .scratch/<owner>/<repo>/<name> of that repository's clone
# .dev/checkouts/<owner>/<repo> to the clone's origin. It also refuses a
# repository that is not owner/repo, or is the queue's own (the one-argument
# form serves that), a clone whose origin names another repository, a
# worktree that is not that clone's, an item whose queue file does not name
# the repository in repos, and a third argument (docs/design/loop-repository.md,
# The native side).
#
# Why it exists: settings.native.json denies every push the session would run
# itself (Bash(git push*), Bash(git -C * push*)), because allow rules for
# pushes admitted force-pushes, deletes and pushes to other branches ("*"
# spans spaces, slashes and colons). It is on main, and native-run.sh refuses
# a branch's tools/workflow/, so no branch changes it before Jeff's merge
# (FLOW-14). It is a guard against an agent's mistake, not a sandbox: make, go
# and powershell can still run git push (docs/design/loop-native-lead.md).
#
# Every refusal is one line on stderr and exit 2, before anything runs;
# otherwise git push is exec'd, so its output and exit status are its own.

refuse() {
	echo "native-push.sh: $1; usage: tools/workflow/native/native-push.sh <name> [<owner>/<repo>]" >&2
	exit 2
}

# norm prints a directory's physical path; git and pwd spell the same
# directory differently (relative, C:/ against /c/, /tmp against /private/tmp).
norm() {
	(cd "$1" 2>/dev/null && pwd -P)
}

top=$(git rev-parse --show-toplevel 2>/dev/null) || refuse "not in a git checkout"
[ "$(norm "$top")" = "$(pwd -P)" ] || refuse "run from the checkout root"

[ $# -eq 1 ] || [ $# -eq 2 ] || refuse "takes the worktree's name and, for a clone, <owner>/<repo>"
name=$1
case $name in
'' | . | .. | */*) refuse "bad worktree name '$name'" ;;
esac

wt=.scratch/$name
common=$(norm "$(git rev-parse --git-common-dir)")
repo=""
if [ $# -eq 2 ]; then
	repo=$2
	owner=${repo%%/*} rname=${repo#*/}
	case $owner/$rname in
	"$repo") ;;
	*) refuse "bad repository '$repo'" ;;
	esac
	for part in "$owner" "$rname"; do
		case $part in
		'' | . | .. | *[!A-Za-z0-9_.-]*) refuse "bad repository '$repo', not <owner>/<repo>" ;;
		esac
	done
	own=$(sed -n 's/^repository: *//p' .dev/workflow/config.yaml 2>/dev/null)
	[ "$repo" != "$own" ] || refuse "$repo is the queue's own repository: push it with the one-argument form"
	clone=.dev/checkouts/$repo
	clone_top=$(git -C "$clone" rev-parse --show-toplevel 2>/dev/null) || refuse "$clone is not a clone"
	[ "$(norm "$clone_top")" = "$(norm "$clone")" ] || refuse "$clone is not a clone's top level"
	url=$(git -C "$clone" remote get-url origin 2>/dev/null) || refuse "$clone has no origin"
	case $url in
	*[/:]"$repo" | *[/:]"$repo".git) ;;
	*) refuse "$clone's origin is $url, not $repo" ;;
	esac
	common=$(cd "$clone" && norm "$(git rev-parse --git-common-dir)")
	wt=.scratch/$repo/$name
fi
wt_top=$(git -C "$wt" rev-parse --show-toplevel 2>/dev/null) || refuse "$wt is not a git worktree"
[ "$(norm "$wt_top")" = "$(norm "$wt")" ] || refuse "$wt is not a worktree's top level"
wt_common=$(cd "$wt" && norm "$(git rev-parse --git-common-dir)") || refuse "$wt is not a git worktree"
[ "$wt_common" = "$common" ] || refuse "$wt is not a worktree of ${repo:-this repository}"

ref=$(git -C "$wt" symbolic-ref -q HEAD) || refuse "$wt has a detached HEAD"
item=${ref#refs/heads/feat/}
[ "$item" != "$ref" ] || refuse "$wt is on $ref, not feat/<item>"
case $item in
[a-z0-9]*) ;;
*) refuse "$wt is on $ref, not feat/<item>" ;;
esac
case $item in
*[!a-z0-9-]*) refuse "$wt is on $ref, not feat/<item>" ;;
esac

queue=.dev/workflow/queue/$item.yaml
[ -f "$queue" ] || refuse "$item is not in the queue ($queue is missing)"
grep -qx 'status: in_progress' "$queue" || refuse "$item is not in progress ($queue)"
if [ -n "$repo" ]; then
	# repos as wf writes it, a block list, or a flow list written by hand.
	awk -v r="$repo" '
		/^repos:/ { inr = 1; line = $0; sub(/^repos:[ ]*/, "", line)
			if (line ~ /^\[/) { gsub(/[][ ]/, "", line); n = split(line, a, ",")
				for (i = 1; i <= n; i++) if (a[i] == r) found = 1; inr = 0 }
			next }
		inr && /^[ ]+- / { v = $0; sub(/^[ ]+- [ ]*/, "", v); if (v == r) found = 1; next }
		{ inr = 0 }
		END { exit !found }' "$queue" || refuse "$item's repos do not name $repo ($queue)"
fi

exec git -C "$wt" push --set-upstream origin "refs/heads/feat/$item:refs/heads/feat/$item"
