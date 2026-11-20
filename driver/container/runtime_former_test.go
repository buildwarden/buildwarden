// TRANSITIONAL(runtime-former-values): removed with runtime_former.go, in
// the change that prepares 1.0.

package container

import (
	"errors"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func TestFormerRuntimeValues(t *testing.T) {
	const (
		otherCase = "the container runtime is named in lower case, without an extension"
		asPath    = "the container runtime is named, not given as a path"
		onPath    = "; warden looks docker up on PATH, so if it is installed " +
			"outside PATH, add its directory to PATH"
		otherRuntime = "the container driver supports finch, docker and podman " +
			"and no longer runs another runtime"
		noNerdctl = "finch, docker or podman; there is no supported way to use " +
			"nerdctl with the container driver"
		otherProgram = "the container driver runs finch, docker or podman by " +
			"name and no longer runs another program"
		noWrapper = "finch, docker or podman; if the program only set variables " +
			"for one of them (DOCKER_HOST, DOCKER_CONTEXT, CONTAINER_HOST), set " +
			"those in the environment; there is no supported way to run another " +
			"program as the runtime"
	)
	for _, tc := range []struct {
		v         string
		what, fix string // both empty: not one of the former forms
	}{
		// A supported runtime in another letter case or with .exe.
		{"Docker", otherCase, "docker"},
		{"docker.exe", otherCase, "docker"},
		{"PODMAN.EXE", otherCase, "podman"},
		// A path whose last element is a supported runtime. No directory of
		// the value is in the fix.
		{"/opt/homebrew/bin/docker", asPath, "docker" + onPath},
		{`C:\Program Files\Docker\docker.exe`, asPath, "docker" + onPath},
		{"./tools/docker", asPath, "docker" + onPath},
		{"C:docker", asPath, "docker" + onPath},
		{"/usr/local/bin/Finch", asPath, "finch; warden looks finch up on PATH, so " +
			"if it is installed outside PATH, add its directory to PATH"},
		// nerdctl, alone or as a path's last element.
		{"nerdctl", otherRuntime, noNerdctl},
		{"/usr/local/bin/nerdctl", otherRuntime, noNerdctl},
		// A path to anything else.
		{"/usr/local/bin/docker-ci", otherProgram, noWrapper},
		{"./rt", otherProgram, noWrapper},
		{`..\x.exe`, otherProgram, noWrapper},
		// A bare name that is none of these: the permanent error.
		{"sh", "", ""},
		{"dokcer", "", ""},
		{" docker", "", ""},
		{"docker ", "", ""},
		{"sudo docker", "", ""},
	} {
		t.Run(tc.v, func(t *testing.T) {
			// The detection only words a refusal: the check has refused
			// every value it is given, recognized or not.
			if name, ok := supportedRuntime(tc.v); ok {
				t.Fatalf("supportedRuntime accepts %q as %q", tc.v, name)
			}
			what, fix, ok := formerRuntimeValue(tc.v)
			if ok != (tc.what != "") || what != tc.what || fix != tc.fix {
				t.Errorf("formerRuntimeValue(%q) = %v\n"+
					"got  %q\n     %q\nwant %q\n     %q",
					tc.v, ok, what, fix, tc.what, tc.fix)
			}
		})
	}
}

// The fix of a former value starts as the permanent one does for its level.
func TestFormerRuntimeValueInTheError(t *testing.T) {
	for _, tc := range []struct {
		source, v string
		msg, fix  string
	}{
		{cliFromEnv, "/opt/homebrew/bin/docker",
			`WARDEN_CTR_CLI is "/opt/homebrew/bin/docker"; ` +
				"the container runtime is named, not given as a path",
			"set WARDEN_CTR_CLI to docker; warden looks docker up on PATH, so " +
				"if it is installed outside PATH, add its directory to PATH"},
		{cliFromFlag, "docker.exe",
			`--runtime is "docker.exe"; the container runtime ` +
				"is named in lower case, without an extension",
			"pass --runtime docker"},
		{"warden.toml", "nerdctl",
			`[runtime] cli in warden.toml is "nerdctl"; the container driver ` +
				"supports finch, docker and podman and no longer runs another " +
				"runtime",
			"set it to finch, docker or podman; there is no supported way to " +
				"use nerdctl with the container driver"},
	} {
		source := tc.source
		if source == "warden.toml" {
			source += " [runtime] cli"
		}
		err := unsupportedRuntime(tc.v, source)
		var fe *driver.FixableError
		if !errors.As(err, &fe) {
			t.Fatalf("%s = %q: %v is not a FixableError", tc.source, tc.v, err)
		}
		if fe.Msg != tc.msg || fe.Fix != tc.fix {
			t.Errorf("%s = %q:\ngot  %q\n     %q\nwant %q\n     %q",
				tc.source, tc.v, fe.Msg, fe.Fix, tc.msg, tc.fix)
		}
	}
}
