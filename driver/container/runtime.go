package container

import (
	"context"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/buildwarden/buildwarden/driver"
)

// runtimes are the runtimes the container driver supports, in the order
// detection prefers them. A runtime named by a flag, a variable, a config
// file or a state file is one of these or is refused (resolveRuntime).
var runtimes = []string{"finch", "docker", "podman"}

// The sources the kit gives the container driver's settings when they were
// not read from a file: the flag and the two variables.
const (
	cliFromFlag       = "--runtime"
	cliFromEnv        = "WARDEN_CTR_CLI"
	relayImageFromEnv = "WARDEN_IMAGE"
)

// keyInFile words a source the kit gives for a key read from a file,
// "<file> [table] key", as the driver's messages put it: "[table] key in
// <file>". Another source is returned as it is.
func keyInFile(source string) string {
	i := strings.LastIndex(source, " [")
	if i < 0 {
		return source
	}
	return source[i+1:] + " in " + source[:i]
}

// supportedRuntime returns the member of runtimes that v equals. The
// comparison is exact, and what is returned is the list's own string, never
// v: no text from a flag, a variable or a file reaches exec.
func supportedRuntime(v string) (string, bool) {
	for _, name := range runtimes {
		if v == name {
			return name, true
		}
	}
	return "", false
}

// runtimeNames lists runtimes in prose, the last after conj: "finch, docker
// or podman".
func runtimeNames(conj string) string {
	last := len(runtimes) - 1
	return strings.Join(runtimes[:last], ", ") + " " + conj + " " + runtimes[last]
}

// resolveRuntime returns the container runtime to run: the one set names
// when it is a supported runtime, the detected one when none does. Another
// value is refused here, before anything is looked up, run or created, and
// so are settings that could not be read: in reap and attach they are
// resolved on this first read (DRV-C10).
func resolveRuntime(set driver.Settings) (string, error) {
	v, _ := set.String("runtime")
	if err := set.Err(); err != nil {
		return "", err
	}
	if v == "" {
		return detectRuntime()
	}
	if name, ok := supportedRuntime(v); ok {
		return name, nil
	}
	return "", unsupportedRuntime(v, set.Source("runtime"))
}

// checkNamedRuntime refuses the runtime rt that set names, when it does,
// unless it is on PATH and answers `<rt> info` within 3 s: a value the user
// provided that cannot be used is an error before any work, never replaced
// by detection (Decision 26, Behaviour 20). A runtime set does not name was
// detected, and detection has made the same checks. run's probe and doctor
// call it; the build itself resolves as resolveRuntime does.
func checkNamedRuntime(rt string, set driver.Settings) error {
	if v, _ := set.String("runtime"); v == "" {
		return nil
	}
	setting, other := namedRuntimeFix(set.Source("runtime"))
	if _, err := exec.LookPath(rt); err != nil {
		return driver.Fixable(runtimeInstallHint(rt)+", "+other,
			"%s names the container runtime %s, which is not installed "+
				"(not found on PATH)", setting, rt)
	}
	if !runtimeAnswers(rt) {
		return driver.Fixable(runtimeStartHint(rt)+", "+other,
			"%s names the container runtime %s, which is installed but not running "+
				"(`%s info` failed)", setting, rt, rt)
	}
	return nil
}

// namedRuntimeFix words where the runtime was named, as checkNamedRuntime's
// message puts it, and the fix that names another there.
func namedRuntimeFix(source string) (setting, other string) {
	switch source {
	case cliFromFlag:
		return cliFromFlag, "or pass --runtime another runtime, or leave it out"
	case cliFromEnv:
		return cliFromEnv, "or set WARDEN_CTR_CLI to another runtime, or unset it"
	}
	return keyInFile(source), "or set it to another runtime, or remove it"
}

// runtimeInstallHint is how to install the runtime name.
func runtimeInstallHint(name string) string {
	switch name {
	case "finch":
		return "install Finch (brew install --cask finch && finch vm init)"
	case "podman":
		return "install Podman"
	default:
		return "install Docker"
	}
}

// unsupportedRuntime is the error for a runtime in effect that is not one of
// runtimes. It names the level that set the value and gives the fix for that
// level. The value is printed once, quoted: it may come from a file someone
// else wrote, and %q keeps control characters out of the terminal. Nothing
// else in the message or the fix is taken from the value.
func unsupportedRuntime(value, source string) error {
	var setting, set, orNot string
	switch source {
	case cliFromFlag:
		setting, set, orNot = cliFromFlag, "pass --runtime ", ", or leave it out"
	case cliFromEnv:
		setting, set, orNot = cliFromEnv, "set WARDEN_CTR_CLI to ", ", or unset it"
	default:
		setting, set = keyInFile(source), "set it to "
		orNot = ", or remove it; --runtime with one of those replaces it " +
			"for one command"
	}
	what := "the container driver supports " + runtimeNames("and")
	fix := set + runtimeNames("or") + orNot
	// TRANSITIONAL(runtime-former-values): a value that ran before this
	// check existed gets a message that says what to change.
	if formerWhat, formerFix, ok := formerRuntimeValue(value); ok {
		what, fix = formerWhat, set+formerFix
	}
	return driver.Fixable(fix, "%s is %q; %s", setting, value, what)
}

// detectRuntime probes for a working container runtime, in runtimes' order.
func detectRuntime() (string, error) {
	var installed []string
	for _, name := range runtimes {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		installed = append(installed, name)
		if runtimeAnswers(name) {
			return name, nil
		}
	}
	if len(installed) == 0 {
		return "", driver.Fixable(
			"install Finch (brew install --cask finch && finch vm init), "+
				"Docker or Podman, or use a VM driver (--driver qemu or vz)",
			"no container runtime found (tried finch, docker, podman)")
	}
	// Installed but not answering: almost always its VM or daemon is down.
	return "", notAnswering(installed[0], strings.Join(installed, ", "))
}

// runtimeAnswers reports whether `<name> info` succeeds within 3 s.
func runtimeAnswers(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, "info").Run() == nil
}

// notAnswering is the error for the runtime name, installed as installed
// says, whose `info` failed.
func notAnswering(name, installed string) error {
	return driver.Fixable(runtimeStartHint(name),
		"container runtime %s is installed but not running (`%s info` failed)",
		installed, name)
}

// runtimeStartHint is the command that starts a stopped runtime.
func runtimeStartHint(name string) string {
	switch name {
	case "finch":
		return "start its VM: finch vm start (or finch vm init the first time)"
	case "podman":
		return "start its machine: podman machine start " +
			"(or podman machine init the first time)"
	default:
		return "start Docker (Docker Desktop, or the docker service)"
	}
}

// checkRelayImage refuses a relay image that is not shaped like an image
// reference. The value goes to `<runtime> pull` and to the container's
// creation as one argument, so one that starts with "-" would be read as an
// option, and a space or a control character has no place in a reference.
// A control character here is any that does not print: the C0 and C1
// controls, and the format characters (a right-to-left override, a
// zero-width space, a byte order mark) that reorder or hide the text around
// them where the value is printed; a byte that is not valid UTF-8 too.
// Empty and "dev" are the release's image and a development build; nothing
// more of a reference is checked here, which the runtime's own error does.
func checkRelayImage(img, source string) error {
	if img == "" || img == "dev" {
		return nil
	}
	bad := strings.HasPrefix(img, "-") || strings.IndexFunc(img, func(r rune) bool {
		return r == ' ' || r == utf8.RuneError || !unicode.IsPrint(r)
	}) >= 0
	if !bad {
		return nil
	}
	return driver.Fixable("set it to an image reference such as "+
		"ghcr.io/buildwarden/relay:v0.2.0 or to \"dev\", or remove it",
		"%s is %q: not an image reference", keyInFile(source), img)
}
