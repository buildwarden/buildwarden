// TRANSITIONAL(runtime-former-values): removed in the change that prepares
// 1.0, together with the call in unsupportedRuntime (config.go),
// runtime_former_test.go, the "A path, or another runtime" subsection of the
// configuration reference (guide/configuration.md) and the upgrade note in
// getting-started/installation.md that links to it. A search for the marker
// finds all five. It also finds docs/design/runtime-cli-project-path.md, the
// archived design that records them, which is kept.
//
// Until the container runtime was limited to containerRuntimes, warden ran
// whatever program the setting named, and four forms of that were in use: a
// supported runtime in another letter case or with .exe, a path to one,
// nerdctl, and a path to any other program. supportedRuntime refuses each
// now. This file gives each a message that says what to change, where the
// permanent error only lists the supported values. It words a refusal that
// has already been made and never changes whether a value is accepted.

package main

import "strings"

// formerRuntimeValue recognizes a refused runtime value in one of the forms
// that ran before, and words the refusal: what says what is wrong with the
// value, and fix follows the level's "set it to". Neither holds text from v.
// A name in them is containerRuntimes' own, and a directory in v is not
// repeated: a project's warden.toml may have chosen it, and warden would be
// telling the user to put a directory someone else named on PATH.
func formerRuntimeValue(v string) (what, fix string, ok bool) {
	last, isPath := lastElement(v)
	program := strings.TrimSuffix(strings.ToLower(last), ".exe")
	name, supported := supportedRuntime(program)
	switch {
	case supported && !isPath:
		return "the container runtime is named in lower case, without an extension",
			name, true
	case supported:
		return "the container runtime is named, not given as a path",
			name + "; warden looks " + name + " up on PATH, so if it is " +
				"installed outside PATH, add its directory to PATH", true
	case program == "nerdctl":
		return "the container driver supports " + runtimeNames("and") +
				" and no longer runs another runtime",
			runtimeNames("or") + "; there is no supported way to use " +
				"nerdctl with the container driver", true
	case isPath:
		return "the container driver runs " + runtimeNames("or") +
				" by name and no longer runs another program",
			runtimeNames("or") + "; if the program only set variables for " +
				"one of them (DOCKER_HOST, DOCKER_CONTEXT, CONTAINER_HOST), " +
				"set those in the environment; there is no supported way to " +
				"run another program as the runtime", true
	}
	return "", "", false
}

// lastElement returns what follows the last separator in v, and whether v
// has one. A separator is /, \ or a drive letter's colon on every OS, not
// the host's own (path/filepath): a value written for one OS gets the same
// message on another.
func lastElement(v string) (string, bool) {
	i := strings.LastIndexAny(v, `/\`)
	if i < 0 && len(v) >= 2 && v[1] == ':' && isASCIILetter(v[0]) {
		i = 1
	}
	return v[i+1:], i >= 0
}

func isASCIILetter(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z'
}
