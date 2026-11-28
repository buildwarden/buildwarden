package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/check"
)

// exitError is a command's failure with an exit status other than 1.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// syncShapeCmd is wf sync-shape, run by CI's program sync step: whether the
// pull request's head is a program sync's (check.SyncShape) whose
// resolution edits no path FLOW-14 guards (check.GuardedResolution), so that
// CI judges it as the push its merge makes (FLOW-01, FLOW-14, FLOW-20). It
// reads no queue. Exit 0 is a sync; 1 is not one, each failed condition
// printed; 2 is a usage or git error. Anything but 0 is judged as any PR.
func syncShapeCmd(args []string) error {
	var repo, base, branch, head, trunk, profile string
	fs := flag.NewFlagSet("sync-shape", flag.ExitOnError)
	fs.StringVar(&repo, "repo", ".", "the checkout, with the PR head and -trunk fetched")
	fs.String("state", ".", "ignored: wf sync-shape reads no queue state")
	fs.StringVar(&base, "base", "", "the pull request's base commit, on the program trunk")
	fs.StringVar(&branch, "branch", "", "the pull request's head branch")
	fs.StringVar(&head, "head", "", "the pull request's head commit")
	fs.StringVar(&trunk, "trunk", "", "the trunk's remote-tracking ref, origin/<trunk>")
	fs.StringVar(&profile, "profile", "", "the profile file, a path in the checkout")
	_ = fs.Parse(args)
	if base == "" || branch == "" || head == "" || trunk == "" || profile == "" {
		return &exitError{2, errors.New(
			"sync-shape needs -base, -branch, -head, -trunk and -profile")}
	}
	res, err := check.SyncShape(repo, base, branch, head, trunk)
	var ns *check.NotSyncError
	switch {
	case errors.As(err, &ns):
		return notSync(head, ns.Why)
	case err != nil:
		return &exitError{2, err}
	}
	guarded, err := check.GuardedResolution(repo, profile, head, res)
	if err != nil {
		return &exitError{2, err}
	}
	if len(guarded) > 0 {
		return notSync(head, []string{fmt.Sprintf("the resolution edits %s, which "+
			"FLOW-14 guards", strings.Join(guarded, ", "))})
	}
	ps, err := git(repo, "rev-parse", head+"^1", head+"^2")
	if err != nil {
		return &exitError{2, err}
	}
	p := strings.Fields(ps)
	fmt.Printf("sync %s merges %s into %s\n", short12(head), short12(p[1]), short12(p[0]))
	if len(res) > 0 {
		fmt.Printf("resolves %s, judged by its tier-2 review (FLOW-08, FLOW-20)\n",
			strings.Join(res, ", "))
	}
	return nil
}

// notSync prints why head is not a program sync's and fails with exit 1.
func notSync(head string, why []string) error {
	for _, w := range why {
		fmt.Printf("not a sync: %s\n", w)
	}
	return fmt.Errorf("%s is not a program sync: the PR is judged as any PR (FLOW-01)",
		short12(head))
}

// short12 is a commit's 12-character short hash.
func short12(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
