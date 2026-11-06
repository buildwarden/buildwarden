#!/bin/sh
# native-run.sh: the one route by which an unattended native session runs a
# branch's own script or sets a WARDEN_* variable (workflow-native skill).
#
#   tools/workflow/native/native-run.sh [WARDEN_X=value ...] -- <command> [args]
#
# Run from the checkout root. Accepted commands:
#   - .scratch/<name>/tools/<path>, with or without assignments: a tracked
#     file of the worktree .scratch/<name>, outside its tools/workflow/;
#   - with at least one assignment, also .scratch/<name>/dist/warden*,
#     dist/warden*, ./warden*, go test ... and go -C .scratch/<name> test ...
#
# Why it exists: settings.native.json allows tools/workflow/*, so this script
# needs no new allow rule. The Claude Code matcher does not match an allow rule
# past an assignment of an unknown variable, and an allow rule for
# .scratch/*/tools/* would match any command starting with .scratch/ that
# contains /tools/ ("*" spans spaces and slashes). The checks below are on the
# command's literal text and the worktree's index; they keep sessions on forms
# the digest can show as the real command. They are not a sandbox: make, go
# and powershell already run anything (docs/design/native-allowlist-scratch-tools.md).
#
# Every refusal is one line on stderr and exit 2, before anything runs;
# otherwise the command is exec'd, so its output and exit status are its own.

shapes='accepted: [WARDEN_X=value ...] -- .scratch/<name>/tools/<path> [args]; WARDEN_X=value ... -- .scratch/<name>/dist/warden*|dist/warden*|./warden*|go test ...|go -C .scratch/<name> test ...'

refuse() {
	echo "native-run.sh: $1; $shapes" >&2
	exit 2
}

# norm prints a directory's physical path; git and pwd spell the same
# directory differently (relative, C:/ against /c/, /tmp against /private/tmp).
norm() {
	(cd "$1" 2>/dev/null && pwd -P)
}

# valid_name accepts one path component other than "", "." and "..".
valid_name() {
	case $1 in
	'' | . | .. | */*) return 1 ;;
	esac
	return 0
}

top=$(git rev-parse --show-toplevel 2>/dev/null) || refuse "not in a git checkout"
[ "$(norm "$top")" = "$(pwd -P)" ] || refuse "run from the checkout root"

# First pass: check the assignments up to "--" and pick out the command's
# first words. Nothing is exported until every check has passed. Secret-looking
# names are refused because the command line is in the transcript (FLOW-18).
nassign=0
dashdash=
cmd=
w2=
w3=
w4=
i=0
for a in "$@"; do
	if [ -n "$dashdash" ]; then
		i=$((i + 1))
		case $i in
		1) cmd=$a ;;
		2) w2=$a ;;
		3) w3=$a ;;
		4) w4=$a ;;
		esac
		continue
	fi
	if [ "$a" = -- ]; then
		dashdash=1
		continue
	fi
	case $a in
	*=*) ;;
	*) refuse "'$a' before -- is not a NAME=value assignment" ;;
	esac
	name=${a%%=*}
	case $name in
	WARDEN_*) ;;
	*) refuse "variable $name is not WARDEN_*" ;;
	esac
	case ${name#WARDEN_} in
	'' | *[!A-Z0-9_]*) refuse "variable $name is not WARDEN_[A-Z0-9_]+" ;;
	esac
	case $name in
	*_PASSWORD | *_TOKEN | *_SECRET | *_KEY) refuse "variable $name looks like a secret" ;;
	esac
	nassign=$((nassign + 1))
done
[ -n "$dashdash" ] || refuse "no -- before the command"
[ "$i" -gt 0 ] || refuse "no command after --"

case $cmd in
.scratch/*/tools/*)
	rest=${cmd#.scratch/}
	name=${rest%%/*}
	path=${rest#"$name"/tools/}
	[ "$path" != "$rest" ] || refuse "$cmd is not .scratch/<name>/tools/<path>"
	valid_name "$name" || refuse "$cmd: bad worktree name"
	# An empty, "." or ".." component could leave the worktree or dodge the
	# workflow/ check below.
	case /$path/ in
	*//* | */./* | */../*) refuse "$cmd has an empty, . or .. component" ;;
	esac
	# A branch's loop tooling reaches the loop only by merge (FLOW-14).
	[ "${path%%/*}" != workflow ] || refuse "$cmd is under the branch's tools/workflow/"
	# The target must be a tracked file of a worktree of this repository, so
	# it is a branch's code and not a file the session wrote. A plain
	# directory under .scratch/ resolves to the main checkout, hence the
	# top-level check.
	wt=.scratch/$name
	wt_top=$(git -C "$wt" rev-parse --show-toplevel 2>/dev/null) || refuse "$wt is not a git worktree"
	[ "$(norm "$wt_top")" = "$(norm "$wt")" ] || refuse "$wt is not a worktree's top level"
	wt_common=$(cd "$wt" && norm "$(git rev-parse --git-common-dir)") || refuse "$wt is not a git worktree"
	[ "$wt_common" = "$(norm "$(git rev-parse --git-common-dir)")" ] || refuse "$wt is not a worktree of this repository"
	git -C "$wt" ls-files --error-unmatch "tools/$path" >/dev/null 2>&1 || refuse "tools/$path is not tracked in $wt"
	;;
*)
	[ "$nassign" -gt 0 ] || refuse "$cmd needs no launcher without a WARDEN_* assignment"
	case $cmd in
	.scratch/*/dist/warden*)
		rest=${cmd#.scratch/}
		name=${rest%%/*}
		bin=${rest#"$name"/dist/}
		valid_name "$name" || refuse "$cmd: bad worktree name"
		case $bin in
		*/*) refuse "$cmd is not .scratch/<name>/dist/warden*" ;;
		esac
		;;
	dist/warden* | ./warden*)
		case ${cmd#*/} in
		*/*) refuse "$cmd is not dist/warden* or ./warden*" ;;
		esac
		;;
	go)
		if [ "$w2" = test ]; then
			:
		elif [ "$w2" = -C ] && [ "$w4" = test ]; then
			case $w3 in
			.scratch/*) valid_name "${w3#.scratch/}" || refuse "go -C $w3: not .scratch/<name>" ;;
			*) refuse "go -C $w3: not .scratch/<name>" ;;
			esac
		else
			refuse "go is accepted only as go test or go -C .scratch/<name> test"
		fi
		;;
	*) refuse "$cmd is not an accepted command" ;;
	esac
	;;
esac

while [ "$1" != -- ]; do
	export "$1"
	shift
done
shift
exec "$@"
