#!/bin/sh
# native-push.sh: the one route by which an unattended native session pushes a
# feature branch (FLOW-17, workflow-native skill).
#
#   tools/workflow/native/native-push.sh <name>
#
# Run from the checkout root. It pushes the branch feat/<item> checked out in
# the worktree .scratch/<name> to the same name on origin, one refspec and no
# "+", so git refuses a non-fast-forward and nothing else moves; a first push
# sets the upstream. It refuses a branch whose item is not in progress in the
# queue state at .dev/workflow, a detached head, any other branch name, and
# any argument after the name.
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
	echo "native-push.sh: $1; usage: tools/workflow/native/native-push.sh <name>" >&2
	exit 2
}

# norm prints a directory's physical path; git and pwd spell the same
# directory differently (relative, C:/ against /c/, /tmp against /private/tmp).
norm() {
	(cd "$1" 2>/dev/null && pwd -P)
}

top=$(git rev-parse --show-toplevel 2>/dev/null) || refuse "not in a git checkout"
[ "$(norm "$top")" = "$(pwd -P)" ] || refuse "run from the checkout root"

[ $# -eq 1 ] || refuse "takes one argument, the worktree's name"
name=$1
case $name in
'' | . | .. | */*) refuse "bad worktree name '$name'" ;;
esac

wt=.scratch/$name
wt_top=$(git -C "$wt" rev-parse --show-toplevel 2>/dev/null) || refuse "$wt is not a git worktree"
[ "$(norm "$wt_top")" = "$(norm "$wt")" ] || refuse "$wt is not a worktree's top level"
wt_common=$(cd "$wt" && norm "$(git rev-parse --git-common-dir)") || refuse "$wt is not a git worktree"
[ "$wt_common" = "$(norm "$(git rev-parse --git-common-dir)")" ] || refuse "$wt is not a worktree of this repository"

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

exec git -C "$wt" push --set-upstream origin "refs/heads/feat/$item:refs/heads/feat/$item"
