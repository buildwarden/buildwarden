package container

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

const relayImageFix = "set it to an image reference such as " +
	"ghcr.io/buildwarden/relay:v0.2.0 or to \"dev\", or remove it"

// Empty, dev and references pass; a value shaped like an option, or holding
// a space or a control character, is refused from either key of the file and
// from WARDEN_IMAGE, by the probe of run and by doctor, before the runtime is
// resolved.
func TestRelayImageShape(t *testing.T) {
	passes := []string{"", "dev", "ghcr.io/buildwarden/relay:v0.2.0",
		"localhost:5000/relay@sha256:0123", "relay"}
	refused := map[string]string{
		"an option":                 "--volume=/:/host",
		"a short option":            "-v",
		"a space":                   "relay --privileged",
		"a tab":                     "relay\t--privileged",
		"a line break":              "relay\nx",
		"an escape character":       "relay\x1b[2J",
		"a right-to-left override":  "a\u202eb",
		"a zero-width space":        "a\u200bb",
		"a leading byte order mark": "\ufeff--x",
		"a no-break space":          "relay\u00a0x",
	}
	// What a refusal of the runtime looks like: no runtime answers in the
	// test, which is not this test's subject.
	isRuntimeError := func(err error) bool {
		return err != nil && !strings.Contains(err.Error(), "relay_image") &&
			!strings.Contains(err.Error(), "WARDEN_IMAGE")
	}
	commands := map[string]func(r *runtimeTest) error{
		"the probe of run": runtimeCommands[0].call,
		"doctor":           runtimeCommands[1].call,
	}
	keys := []struct{ table, label string }{
		{"runtime", "[runtime] relay_image"},
		{"driver.container", "[driver.container] relay_image"},
	}
	for _, k := range keys {
		for _, v := range passes {
			t.Run(k.label+" passes "+v, func(t *testing.T) {
				r := newRuntimeTest(t)
				// Detection finds nothing, on any host: a real runtime's
				// version query, killed at its timeout, can leave a child
				// holding the test's directory (windows-latest).
				t.Setenv("PATH", t.TempDir())
				r.configure(relayImageFile(k.table, v))
				err := runtimeCommands[0].call(r)
				if err != nil && !isRuntimeError(err) {
					t.Errorf("probe: %v", err)
				}
			})
		}
	}
	for name, v := range refused {
		for cmd, call := range commands {
			for _, k := range keys {
				t.Run(cmd+" refuses "+name+" from "+k.label, func(t *testing.T) {
					r := newRuntimeTest(t)
					r.configure(relayImageFile(k.table, v))
					msg := r.inFile(k.label) + " is " + strconv.Quote(v) +
						": not an image reference"
					wantFixable(t, call(r), msg, relayImageFix)
				})
			}
			t.Run(cmd+" refuses "+name+" from WARDEN_IMAGE", func(t *testing.T) {
				r := newRuntimeTest(t)
				r.configure("[driver.container]\nrelay_image = \"dev\"\n")
				t.Setenv("WARDEN_IMAGE", v)
				wantFixable(t, call(r), "WARDEN_IMAGE is "+strconv.Quote(v)+
					": not an image reference", relayImageFix)
			})
		}
	}
}

func relayImageFile(table, v string) string {
	return "[" + table + "]\nrelay_image = " + tomlString(v) + "\n"
}

// wantFixable checks that err is a FixableError with msg and fix.
func wantFixable(t *testing.T, err error, msg, fix string) {
	t.Helper()
	var fe *driver.FixableError
	if !errors.As(err, &fe) {
		t.Fatalf("got %v, want a FixableError saying %q", err, msg)
	}
	if fe.Msg != msg || fe.Fix != fix {
		t.Errorf("got  %q\n     %q\nwant %q\n     %q", fe.Msg, fe.Fix, msg, fix)
	}
}
