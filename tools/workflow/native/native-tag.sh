#!/bin/sh
# native-tag.sh: the one route by which a native session creates a tag the
# loop recorded as due (FLOW-17, workflow-native skill).
#
#   tools/workflow/native/native-tag.sh <owner>/<repo> <tag>
#
# Run from the checkout root. A cloud session cannot push a tag, so it
# records each tag wf tag-next names as due, at the commit it checked (a
# tag-due record); this script creates one of them. It reads the tag's line
# from dist/wf tags-due -state .dev/workflow, fetches the clone's origin with
# its tags, creates an annotated tag at the due commit and pushes it, one
# refspec and no "+", so git refuses to replace a tag that appeared
# meanwhile. The clone is the checkout itself for the queue's own repository
# and .dev/checkouts/<owner>/<repo> for another. It refuses a repository that
# is not owner/repo, a malformed tag, a tag wf tags-due does not list, a
# clone whose origin names another repository, a due commit that is not in
# origin/<program trunk>, a tag the remote already has (the refusal names the
# commit it is at, so the session can tell another machine's creation from a
# conflict), a local tag of that name at another commit, and any argument
# after the tag (docs/design/program-autonomy.md, D3).
#
# Why it exists: settings.native.json denies every push the session would run
# itself (Bash(git push*), Bash(git -C * push*)), because "*" spans spaces, so
# an allow rule for tag pushes would admit --force, deletes and other refs, as
# for native-push.sh. It is on main, and native-run.sh refuses a branch's
# tools/workflow/, so no branch changes it before Jeff's merge (FLOW-14). It
# is a guard against an agent's mistake, not a sandbox.
#
# Every refusal is one line on stderr and exit 2, before the tag is created;
# otherwise git push is exec'd, so its output and exit status are its own.

refuse() {
	echo "native-tag.sh: $1; usage: tools/workflow/native/native-tag.sh <owner>/<repo> <tag>" >&2
	exit 2
}

# norm prints a directory's physical path; git and pwd spell the same
# directory differently (relative, C:/ against /c/, /tmp against /private/tmp).
norm() {
	(cd "$1" 2>/dev/null && pwd -P)
}

top=$(git rev-parse --show-toplevel 2>/dev/null) || refuse "not in a git checkout"
[ "$(norm "$top")" = "$(pwd -P)" ] || refuse "run from the checkout root"

[ $# -eq 2 ] || refuse "takes the repository and the tag"
repo=$1 tag=$2
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
case $tag in
'' | -* | /* | */ | *..* | *//* | *.lock | *[!A-Za-z0-9._/-]*) refuse "bad tag '$tag'" ;;
esac

own=$(sed -n 's/^repository: *//p' .dev/workflow/config.yaml 2>/dev/null)
clone=.dev/checkouts/$repo
[ "$repo" != "$own" ] || clone=.
clone_top=$(git -C "$clone" rev-parse --show-toplevel 2>/dev/null) || refuse "$clone is not a clone"
[ "$(norm "$clone_top")" = "$(norm "$clone")" ] || refuse "$clone is not a clone's top level"
url=$(git -C "$clone" remote get-url origin 2>/dev/null) || refuse "$clone has no origin"
case $url in
*[/:]"$repo" | *[/:]"$repo".git) ;;
*) refuse "$clone's origin is $url, not $repo" ;;
esac

[ -x dist/wf ] || refuse "dist/wf is not built"
due=$(dist/wf tags-due -state .dev/workflow 2>/dev/null) || refuse "wf tags-due failed"
line=$(printf '%s\n' "$due" | awk -F '\t' -v r="$repo" -v t="$tag" '$1 == r && $2 == t { print; exit }')
[ -n "$line" ] || refuse "$tag is not due in $repo (wf tags-due)"
tab=$(printf '\t')
IFS=$tab read -r _ _ sha trunk at <<LINE
$line
LINE
case $sha in
'' | *[!0-9a-f]*) refuse "the due commit '$sha' of $tag is not a commit id" ;;
esac
case $trunk in
'' | -) refuse "$repo has no program trunk recorded" ;;
esac

git -C "$clone" fetch -q --tags origin "+refs/heads/$trunk:refs/remotes/origin/$trunk" 2>/dev/null ||
	refuse "fetch of $repo's origin failed"
git -C "$clone" merge-base --is-ancestor "$sha" "refs/remotes/origin/$trunk" 2>/dev/null ||
	refuse "$sha is not in origin/$trunk"
remote=$(git -C "$clone" ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}" 2>/dev/null) ||
	refuse "cannot read $repo's tags"
if [ -n "$remote" ]; then
	# An annotated tag's line names the tag object; its ^{} line, the commit.
	at_commit=$(printf '%s\n' "$remote" | awk '$2 ~ /\^\{\}$/ { c = $1 } NR == 1 { f = $1 } END { print (c != "" ? c : f) }')
	refuse "the remote already has $tag at $at_commit (due at $sha)"
fi
if have=$(git -C "$clone" rev-parse -q --verify "refs/tags/$tag^{commit}"); then
	[ "$have" = "$sha" ] || refuse "a local tag $tag is at $have, not $sha"
else
	git -C "$clone" tag -a "$tag" "$sha" -m "$tag: due at $at, wf tag-next" || exit 1
fi

exec git -C "$clone" push origin "refs/tags/$tag:refs/tags/$tag"
