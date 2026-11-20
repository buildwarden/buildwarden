package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/buildwarden/buildwarden/tools/workflow/check"
	"github.com/buildwarden/buildwarden/tools/workflow/queue"
)

// native is wf native, the native wrapper's and the install scripts' reading
// of the repositories a machine lists in hosts.yaml (a14 of
// docs/design/loop-repository.md). -repo is the machine's dedicated
// checkout; a second repository's clone is .dev/checkouts/<owner>/<name> in
// it.
//
//   - -clones prints "<owner>/<name>\t<trunk>" for each listed repository
//     other than the queue's own: the clones the wrapper resets to their
//     trunk.
//   - -prepare runs, for each host name the machine serves, the native.prepare
//     commands of each listed repository whose profile file names it, in that
//     repository's checkout. A command that fails prints
//     "stale\t<host>\t<why>": the wrapper skips that host's steps this run.
//     It fails only when it cannot run at all.
//   - -check is the install check (LOOP-05): each clone is there, its origin
//     is the listed repository and it is clean; each profile's
//     toolchain_check passes; each served host's native.check commands run,
//     a failure a warning (the run's native.prepare rebuilds the asset). A
//     missing clone is reported with the git clone command for the owner to
//     run; nothing is cloned or installed.
func native(s *queue.State, o opts) error {
	if o.machine == "" {
		return errors.New("native needs -machine")
	}
	m, err := s.Machine(o.machine)
	if err != nil {
		return err
	}
	switch {
	case o.clones:
		for _, r := range s.Lists(m) {
			if r == s.Config.Repository {
				continue
			}
			trunk, err := s.Config.Trunk(r)
			if err != nil {
				return err
			}
			fmt.Printf("%s\t%s\n", r, trunk)
		}
		return nil
	case o.prepare:
		return nativePrepare(s, m, o)
	case o.check:
		return nativeCheck(s, m, o)
	}
	return errors.New("native needs -clones, -prepare or -check")
}

// listedCheckout is one repository the machine lists, as the native side
// reads it.
type listedCheckout struct {
	name, dir string
	profile   *check.Profile
}

// listedCheckouts returns each repository the machine lists, with its
// checkout and, for one whose config.yaml entry names a profile, its profile
// file as the checkout has it, at its trunk after the wrapper's reset.
func listedCheckouts(s *queue.State, m *queue.Machine, o opts) ([]listedCheckout,
	error,
) {
	var out []listedCheckout
	for _, r := range s.Lists(m) {
		c := listedCheckout{name: r, dir: o.repo}
		if r != s.Config.Repository {
			c.dir = filepath.Join(o.repo, ".dev", "checkouts", filepath.FromSlash(r))
		}
		e := s.Config.Entry(r)
		if e == nil {
			return nil, fmt.Errorf("config.yaml's repositories has no entry for %s", r)
		}
		if e.Profile != "" {
			path := filepath.Join(c.dir, filepath.FromSlash(e.Profile))
			p, err := check.ReadProfile(path)
			if err == nil {
				err = p.CheckTrunk(r+":"+e.Profile, e.Trunk)
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", r, err)
			}
			c.profile = p
		}
		out = append(out, c)
	}
	return out, nil
}

// runIn runs one command in dir and returns its output's last line on a
// failure.
func runIn(dir string, argv []string) error {
	if len(argv) == 0 {
		return errors.New("an empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", strings.Join(argv, " "), err)
	}
	return nil
}

// nativePrepare is wf native -prepare. A host's prepare that fails stales
// only that host for the run (Defaults and overrides).
func nativePrepare(s *queue.State, m *queue.Machine, o opts) error {
	cs, err := listedCheckouts(s, m, o)
	if err != nil {
		return err
	}
	for _, h := range m.Hosts {
		for _, c := range cs {
			if c.profile == nil {
				continue
			}
			for _, argv := range c.profile.Native.Prepare[h] {
				if err := runIn(c.dir, argv); err != nil {
					fmt.Printf("stale\t%s\t%s in %s failed (native.prepare)\n",
						h, strings.Join(argv, " "), c.name)
					break
				}
				fmt.Printf("prepared\t%s\t%s in %s\n", h, strings.Join(argv, " "),
					c.name)
			}
		}
	}
	return nil
}

// nativeCheck is wf native -check, the install scripts' check of the clones
// and the profiles' commands. It prints ok, warn and FAIL lines and fails
// when any check does.
func nativeCheck(s *queue.State, m *queue.Machine, o opts) error {
	c := &installCheck{}
	present := c.clones(s, m, o)
	if c.problems == 0 {
		cs, err := listedCheckouts(s, m, o)
		if err != nil {
			return err
		}
		for _, l := range cs {
			if l.profile != nil && slices.Contains(present, l.name) {
				c.profile(l, m)
			}
		}
	}
	if c.problems > 0 {
		return fmt.Errorf("%d problems", c.problems)
	}
	return nil
}

// installCheck counts the problems wf native -check reports.
type installCheck struct{ problems int }

func (c *installCheck) line(word, format string, a ...any) {
	fmt.Printf("  %s  %s\n", word, fmt.Sprintf(format, a...))
}

func (c *installCheck) report(ok bool, format string, a ...any) {
	if ok {
		c.line("ok  ", format, a...)
		return
	}
	c.problems++
	c.line("FAIL", format, a...)
}

// clones checks each listed repository's clone and returns the
// repositories whose checkout is there and names them.
func (c *installCheck) clones(s *queue.State, m *queue.Machine, o opts) []string {
	var present []string
	for _, r := range s.Lists(m) {
		if r == s.Config.Repository {
			present = append(present, r)
			continue
		}
		dir := filepath.Join(o.repo, ".dev", "checkouts", filepath.FromSlash(r))
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			c.report(false, "no clone of %s: run git clone "+
				"https://github.com/%s.git %s, and install its toolchain", r, r,
				filepath.ToSlash(dir))
			continue
		}
		url, _ := git(dir, "remote", "get-url", "origin")
		bare := strings.TrimSuffix(url, ".git")
		named := strings.HasSuffix(bare, "/"+r) || strings.HasSuffix(bare, ":"+r)
		c.report(named, "the clone %s has origin %s", filepath.ToSlash(dir), url)
		st, err := git(dir, "status", "--porcelain", "--untracked-files=no")
		c.report(err == nil && st == "", "the clone of %s is clean", r)
		if named {
			present = append(present, r)
		}
	}
	return present
}

// profile runs the checkout's toolchain_check, which must pass, and the
// native.check of each host the machine serves. As before the profile held
// it, a failed native.check warns: the run's native.prepare rebuilds the
// asset.
func (c *installCheck) profile(l listedCheckout, m *queue.Machine) {
	if tc := l.profile.ToolchainCheck; len(tc) > 0 {
		c.report(runIn(l.dir, tc) == nil, "%s toolchain: %s", l.name,
			strings.Join(tc, " "))
	}
	for _, h := range m.Hosts {
		for _, argv := range l.profile.Native.Check[h] {
			word := "ok  "
			if runIn(l.dir, argv) != nil {
				word = "warn"
			}
			c.line(word, "%s %s: %s", l.name, h, strings.Join(argv, " "))
		}
	}
}
