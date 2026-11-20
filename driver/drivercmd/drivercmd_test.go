package drivercmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/buildwarden/buildwarden/driver"
)

// The test binary is also a driver: with testDriverEnv set it runs Main
// on the scenario it names, so that what only a process shows (its
// descriptors, its signals) is tested on a real one.
const testDriverEnv = "DRIVERCMD_TEST_DRIVER"

func TestMain(m *testing.M) {
	switch os.Getenv(testDriverEnv) {
	case "":
		os.Exit(m.Run())
	case "child":
		// A child of the driver that prints to the stdout it inherits.
		fmt.Println("child stray line")
		os.Exit(0)
	case "stray":
		Main(strayDriver())
	case "teardown":
		Main(teardownDriver())
	}
	os.Exit(99)
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// nullHost is a driver.Host that claims at once and never stops.
type nullHost struct{}

func (*nullHost) ClaimRelay(context.Context, driver.RelayStarted) error { return nil }
func (*nullHost) Started() error                                        { return nil }
func (*nullHost) Exited(driver.ExitedPart, string)                      {}
func (*nullHost) Wait(context.Context) string {
	return driver.StopCompleted
}

// kitDriver is a driver with every command, whose functions report what
// they were given.
func kitDriver(t *testing.T) *Driver {
	t.Helper()
	return &Driver{
		Describe: Describe{Name: "kit", Version: "1.2.3", Vendor: "test",
			BugURL: "https://example.invalid/bugs",
			Capabilities: Capabilities{Shell: true, Keep: "none", Exited: true,
				Containerfile: "translate", GuestOS: []string{"linux"},
				ImageOS: []string{"linux"}, DefaultImageOS: "linux"},
			Settings: []Setting{
				{Name: "greeting", Type: String, Flag: "greeting",
					Key:      "driver.kit.greeting",
					Commands: EveryCommand, Usage: "a greeting"},
				{Name: "force", Type: Bool, Flag: "force",
					Commands: []string{CmdImagePrepare}, Usage: "re-acquire"},
				{Name: "image-refresh-after", Type: Age,
					Common:   "image_refresh_after",
					Commands: []string{CmdRun}, Usage: "refresh age"},
			},
			Ext:  []ExtCommand{{Path: []string{"echo"}, Usage: "echo its arguments"}},
			Help: map[string]string{CmdDoctor: "Checks nothing."}},
		Preflight: func(context.Context, *driver.Start) error { return nil },
		Run: func(ctx context.Context, start *driver.Start, host driver.Host) ([]string,
			error) {
			if err := host.ClaimRelay(ctx, driver.RelayStarted{
				Claim: driver.ClaimEndpoint{Network: driver.ClaimTCP,
					Address: "127.0.0.1:1"},
				GuestOS: "linux"}); err != nil {
				return nil, err
			}
			if err := host.Started(); err != nil {
				return nil, err
			}
			host.Wait(ctx)
			return []string{"kept-" + start.BuildID}, nil
		},
		Reap: func(_ context.Context, req *ReapRequest) (int, error) {
			if req.BuildID == "failfail" {
				return 0, driver.Fixable("start the runtime",
					"the runtime does not answer")
			}
			return 4, nil
		},
		Doctor: func(_ context.Context, req *DoctorRequest) (bool, error) {
			fmt.Fprintln(req.stdout(), "ok")
			return true, nil
		},
		Setup: func(context.Context, *SetupRequest) error { return errors.New("boom") },
		Prepare: func(_ context.Context, req *PrepareRequest) (string, error) {
			return "/images/" + req.OS, nil
		},
		List: func(context.Context, *ListRequest) (int, error) { return 2, nil },
		Verify: func(context.Context, *VerifyRequest) (int, int, error) {
			return 3, 1,
				nil
		},
		Clean: func(_ context.Context, req *CleanRequest) (int, error) {
			return len(req.KeepIDs), &Error{Code: CodeBusy, Msg: "another clean runs"}
		},
		Attach: func(context.Context, *AttachRequest) (int, error) { return 7, nil },
		Ext: func(_ context.Context, req *ExtRequest, args []string) error {
			v, _ := req.Settings.String("greeting")
			fmt.Fprintln(req.stdout(), strings.Join(append([]string{v}, args...), " "))
			return nil
		},
	}
}

// invoke runs one command of d as a driver process would, with the request
// on stdin (or in the request file), and returns the exit code, stdout and
// the result file's object.
func invoke(t *testing.T, d *Driver, args []string, request string,
	env ...string) (int, string, map[string]any) {
	t.Helper()
	dir := t.TempDir()
	result := filepath.Join(dir, "result.json")
	env = append(env, EnvResult+"="+result)
	stdin := strings.NewReader(request)
	if args[0] == CmdAttach || args[0] == CmdExt {
		reqFile := filepath.Join(dir, "request.json")
		if err := os.WriteFile(reqFile, []byte(request), 0o600); err != nil {
			t.Fatal(err)
		}
		env = append(env, EnvRequest+"="+reqFile)
		stdin = strings.NewReader("")
	}
	var stdout, stderr bytes.Buffer
	code := runMain(d, args, env, stdin, &stdout, &stderr)
	var res map[string]any
	if data, err := os.ReadFile(result); err == nil {
		if err := json.Unmarshal(data, &res); err != nil {
			t.Fatalf("result file %q: %v", data, err)
		}
	}
	return code, stdout.String(), res
}

// DRV-14, DRV-16, DRV-21, DRV-24 to DRV-29, DRV-65, DRV-66: each command is
// dispatched to its function, and its result and exit code are the
// contract's, an error object included.
func TestDispatch(t *testing.T) {
	d := kitDriver(t)
	state := `"state_file": "/s/build.state"`
	for _, tc := range []struct {
		name    string
		args    []string
		request string
		code    int
		result  string
		stdout  string
	}{
		{"reap", []string{CmdReap, "k3v9x2ab"},
			`{"contract":1,"build_id":"k3v9x2ab",` + state + `}`, 0, `{"removed":4}`,
			""},
		{"reap that cannot look", []string{CmdReap, "failfail"},
			`{"contract":1,"build_id":"failfail",` + state + `}`, 1,
			`{"error":{"code":"fixable","fix":"start the runtime",` +
				`"msg":"the runtime does not answer"}}`, ""},
		{"doctor", []string{CmdDoctor}, `{"contract":1}`, 0, `{"blocked":true}`, "ok\n"},
		{"setup", []string{CmdSetup}, `{"contract":1}`, 1,
			`{"error":{"code":"internal","msg":"boom"}}`, ""},
		{"image-prepare", []string{CmdImagePrepare}, `{"contract":1,"os":"linux"}`, 0,
			`{"path":"/images/linux"}`, ""},
		{"image-list", []string{CmdImageList}, `{"contract":1}`, 0, `{"count":2}`, ""},
		{"image-verify", []string{CmdImageVerify}, `{"contract":1}`, 0,
			`{"checked":3,"drift":1}`, ""},
		{"clean busy", []string{CmdClean}, `{"contract":1,"keep_ids":["k3v9x2ab"]}`, 1,
			`{"error":{"code":"busy","msg":"another clean runs"}}`, ""},
		{"attach", []string{CmdAttach, "k3v9x2ab"},
			`{"contract":1,"build_id":"k3v9x2ab",` + state + `}`, 7, `{}`, ""},
		{"ext", []string{CmdExt, "echo", "a"}, `{"contract":1}`, 0, ``, " echo a\n"},
		{"probe of clean", []string{CmdClean}, `{"contract":1,"keep_ids":[],"probe":true}`,
			0,
			`{"removed":0}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, res := invoke(t, d, tc.args, tc.request)
			if code != tc.code {
				t.Errorf("exit %d, want %d", code, tc.code)
			}
			if stdout != tc.stdout {
				t.Errorf("stdout %q, want %q", stdout, tc.stdout)
			}
			got := ""
			if res != nil {
				b, _ := json.Marshal(res)
				got = string(b)
			}
			if got != tc.result {
				t.Errorf("result %s, want %s", got, tc.result)
			}
		})
	}
}

// DRV-21, DRV-23, DRV-69, DRV-70: the exit codes of a request the driver
// cannot take, each with its error object.
func TestRefusals(t *testing.T) {
	d := kitDriver(t)
	d.Setup, d.Prepare = nil, nil
	for _, tc := range []struct {
		name, command, request string
		code                   int
		errCode                string
	}{
		{"not JSON", CmdDoctor, `not json`, 2, CodeInvalidRequest},
		{"not an object", CmdDoctor, `[1]`, 2, CodeInvalidRequest},
		{"no contract", CmdDoctor, `{}`, 2, CodeInvalidRequest},
		{"wrong field type", CmdDoctor, `{"contract":1,"verbose":"yes"}`, 2,
			CodeInvalidRequest},
		{"missing field", CmdReap, `{"contract":1,"build_id":"k3v9x2ab"}`, 2,
			CodeInvalidRequest},
		{"bad build ID", CmdReap, `{"contract":1,"build_id":"X","state_file":"/s"}`, 2,
			CodeInvalidRequest},
		{"no common version", CmdDoctor, `{"contract":99}`, 4, CodeUnsupportedContract},
		{"not listed", CmdSetup, `{"contract":1}`, 3, CodeUnsupportedCommand},
		// DRV-21: a command the driver does not list exits 3 whatever its
		// request lacks; image-prepare's requires os.
		{"not listed, a field missing", CmdImagePrepare, `{"contract":1}`, 3,
			CodeUnsupportedCommand},
		{"not a command", "frobnicate", `{"contract":1}`, 3, CodeUnsupportedCommand},
		{"undeclared flag", CmdDoctor, `{"contract":1,"args":["--nope=1"]}`, 1,
			CodeFixable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, res := invoke(t, d, []string{tc.command}, tc.request)
			if code != tc.code {
				t.Errorf("exit %d, want %d", code, tc.code)
			}
			e, _ := res["error"].(map[string]any)
			if e["code"] != tc.errCode {
				t.Errorf("error %v, want code %s", res, tc.errCode)
			}
		})
	}
}

// DRV-34: ext started with no request runs with no config file and the
// defaults, and does not exit 2.
func TestExtWithoutRequest(t *testing.T) {
	d := kitDriver(t)
	var stdout bytes.Buffer
	code := runMain(d, []string{CmdExt, "echo", "x"}, []string{}, strings.NewReader(""),
		&stdout, io.Discard)
	if code != 0 || stdout.String() != " echo x\n" {
		t.Errorf("exit %d, stdout %q", code, stdout.String())
	}
}

// contractSchema is the definition def of the contract's schema.
func contractSchema(t *testing.T, def string) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("../../specs/driver-contract/v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	m, _ := doc.(map[string]any)
	id, _ := m["$id"].(string)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(id + "#/$defs/" + def)
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

// DRV-36, DRV-37: describe is built from the Go value, is one JSON object
// valid against the schema, and is the same each time.
func TestDescribeMatchesSchema(t *testing.T) {
	sch := contractSchema(t, "describe")
	minimal := &Driver{Describe: Describe{Name: "min", Version: "0",
		Capabilities: Capabilities{Keep: "none", Containerfile: "ignore",
			GuestOS: []string{"linux"}}},
		Run: func(context.Context, *driver.Start, driver.Host) ([]string,
			error) {
			return nil, nil
		},
		Reap: func(context.Context, *ReapRequest) (int, error) { return 0, nil }}
	for name, d := range map[string]*Driver{"every command": kitDriver(t),
		"minimal": minimal} {
		t.Run(name, func(t *testing.T) {
			var out1, out2 bytes.Buffer
			if code := runMain(d, []string{CmdDescribe}, nil, nil, &out1,
				io.Discard); code != 0 {
				t.Fatalf("describe exit %d", code)
			}
			runMain(d, []string{CmdDescribe}, nil, nil, &out2, io.Discard)
			if out1.String() != out2.String() {
				t.Error("describe differs between two calls")
			}
			if strings.Count(out1.String(), "\n") != 1 {
				t.Errorf("describe prints %q, want one line", out1.String())
			}
			v, err := jsonschema.UnmarshalJSON(&out1)
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(v); err != nil {
				t.Errorf("describe is not valid: %v", err)
			}
		})
	}
	var desc Description
	var out bytes.Buffer
	runMain(kitDriver(t), []string{CmdDescribe}, nil, nil, &out, io.Discard)
	if err := json.Unmarshal(out.Bytes(), &desc); err != nil {
		t.Fatal(err)
	}
	want := `[{"name":"greeting","value":true,` +
		`"commands":["run","doctor","setup","image-prepare","clean"],` +
		`"usage":"a greeting"},` +
		`{"name":"force","value":false,"commands":["image-prepare"],"usage":"re-acquire"}]`
	if b, _ := json.Marshal(desc.Flags); string(b) != want {
		t.Errorf("flags %s\nwant %s", b, want)
	}
	if len(desc.Keys) != 2 || desc.Keys[1].Key != "driver.image_refresh_after" ||
		desc.Keys[0].Commands[len(desc.Keys[0].Commands)-1] != CmdExt {
		t.Errorf("keys %+v", desc.Keys)
	}
}

// DRV-C1, DRV-37: describe refuses what it could not report truthfully.
func TestDescribeRefuses(t *testing.T) {
	for name, edit := range map[string]func(*Driver){
		"reserved flag": func(d *Driver) {
			d.Describe.Settings = append(d.Describe.Settings, Setting{Name: "o",
				Type: String,
				Flag: "output", Commands: []string{CmdRun}, Usage: "u"})
		},
		"flag of a command not listed": func(d *Driver) {
			d.Setup = nil
			d.Describe.Settings = append(d.Describe.Settings, Setting{Name: "s",
				Type: Int,
				Flag: "slots", Commands: []string{CmdSetup}, Usage: "u"})
		},
		"shell without attach": func(d *Driver) { d.Attach = nil },
		"ext without commands": func(d *Driver) { d.Describe.Ext = nil },
		"image_os without prepare": func(d *Driver) {
			d.Prepare = nil
			d.Describe.Settings = d.Describe.Settings[:1]
		},
		"no reap": func(d *Driver) { d.Reap = nil },
	} {
		t.Run(name, func(t *testing.T) {
			d := kitDriver(t)
			edit(d)
			var out bytes.Buffer
			if code := runMain(d, []string{CmdDescribe}, nil, nil, &out,
				io.Discard); code == 0 {
				t.Errorf("describe exit 0: %s", out.String())
			}
		})
	}
}

// ErrorObject maps a fixable error with its fix, an *Error as it is, and
// anything else as internal; their exit codes are the contract's.
func TestErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want Error
		code int
	}{
		{driver.Fixable("do x", "y is wrong"), Error{Code: CodeFixable, Msg: "y is wrong",
			Fix: "do x"}, 1},
		{fmt.Errorf("wrapped: %w", driver.Fixable("f", "m")), Error{Code: CodeFixable,
			Msg: "m", Fix: "f"}, 1},
		{errors.New("boom"), Error{Code: CodeInternal, Msg: "boom"}, 1},
		{&Error{Code: CodeBusy, Msg: "b"}, Error{Code: CodeBusy, Msg: "b"}, 1},
		{&Error{Code: CodeInvalidRequest, Msg: "i"}, Error{Code: CodeInvalidRequest,
			Msg: "i"}, 2},
	} {
		got := ErrorObject(tc.err)
		if got.Code != tc.want.Code || got.Msg != tc.want.Msg || got.Fix != tc.want.Fix {
			t.Errorf("ErrorObject(%v) = %+v, want %+v", tc.err, got, tc.want)
		}
		if exitCode(got.Code) != tc.code {
			t.Errorf("exit code of %s = %d, want %d", got.Code, exitCode(got.Code),
				tc.code)
		}
	}
}

// runPipe starts run of d in this process over pipes and returns the
// orchestrator's ends.
func runPipe(t *testing.T, d *Driver) (in io.WriteCloser, out *bufio.Reader, done <-chan int) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ch := make(chan int, 1)
	go func() {
		code := runMain(d, []string{CmdRun}, []string{}, inR, outW, io.Discard)
		_ = outW.Close()
		ch <- code
	}()
	return inW, bufio.NewReader(outR), ch
}

const startLine = `{"type":"start","contract":1,"build_id":"k3v9x2ab","mode":"build",` +
	`"claim_token":"t","guest_os":"linux","context_dir":"/c","build_script":"/s/b.sh",` +
	`"diagnostics_dir":"/o","state_file":"/l/s.state","args":[]}` + "\n"

func readType(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("reading a message: %v", err)
	}
	var m struct{ Type string }
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("%q is not a message: %v", line, err)
	}
	return m.Type
}

// DRV-45, DRV-47, DRV-49: the messages of a build, in order.
func TestRunSequence(t *testing.T) {
	in, out, done := runPipe(t, kitDriver(t))
	_, _ = io.WriteString(in, startLine)
	if got := readType(t, out); got != "relay-started" {
		t.Fatalf("first message %s", got)
	}
	_, _ = io.WriteString(in, `{"type":"unknown-type"}`+"\n"+`{"type":"claimed"}`+"\n")
	if got := readType(t, out); got != "environment-started" {
		t.Fatalf("second message %s", got)
	}
	_, _ = io.WriteString(in, `{"type":"stop","reason":"completed","detail":"x"}`+"\n")
	line, _ := out.ReadString('\n')
	if line != `{"type":"stopped","kept":["kept-k3v9x2ab"]}`+"\n" {
		t.Errorf("last message %q", line)
	}
	if code := <-done; code != 0 {
		t.Errorf("exit %d", code)
	}
}

// DRV-50: the end of input is a stop, and cancels the build's context; a
// run whose input ends before start writes nothing.
func TestRunEndOfInput(t *testing.T) {
	var sawCancel bool
	d := kitDriver(t)
	d.Run = func(ctx context.Context, _ *driver.Start, host driver.Host) ([]string, error) {
		if err := host.ClaimRelay(ctx, driver.RelayStarted{GuestOS: "linux",
			Claim: driver.ClaimEndpoint{Network: "tcp",
				Address: "127.0.0.1:1"}}); err == nil {
			return nil, errors.New("claimed with no claimed message")
		}
		if reason := host.Wait(ctx); reason != driver.StopInterrupted {
			return nil, fmt.Errorf("Wait = %q", reason)
		}
		<-ctx.Done()
		sawCancel = true
		return nil, nil
	}
	in, out, done := runPipe(t, d)
	_, _ = io.WriteString(in, startLine)
	if got := readType(t, out); got != "relay-started" {
		t.Fatalf("first message %s", got)
	}
	_ = in.Close()
	if got := readType(t, out); got != "stopped" {
		t.Errorf("last message %s", got)
	}
	if code := <-done; code != 0 || !sawCancel {
		t.Errorf("exit %d, context cancelled %v", code, sawCancel)
	}

	var stdout bytes.Buffer
	if code := runMain(d, []string{CmdRun}, nil, strings.NewReader(""), &stdout,
		io.Discard); code != 0 || stdout.Len() != 0 {
		t.Errorf("run with no start: exit %d, stdout %q", code, stdout.String())
	}
}

// DRV-30, DRV-32, DRV-68: a probe of run answers with one message, failed
// with a fixable error for a bad value, without calling Run.
func TestRunProbe(t *testing.T) {
	d := kitDriver(t)
	d.Run = nil
	d.Describe.Commands = []string{CmdRun, CmdReap}
	d.Describe.Capabilities.Shell = false
	d.Describe.Capabilities.ImageOS = nil
	d.Describe.Ext = nil
	d.Describe.Settings = d.Describe.Settings[:1]
	probe := `{"type":"start","contract":1,"probe":true,"mode":"build","guest_os":"linux",` +
		`"context_dir":"/c","args":%s}` + "\n"
	var out bytes.Buffer
	if code := runMain(d, []string{CmdRun}, nil, strings.NewReader(fmt.Sprintf(probe, `[]`)),
		&out, io.Discard); code != 0 || out.String() != `{"type":"stopped"}`+"\n" {
		t.Errorf("probe: exit %d, %q", code, out.String())
	}
	out.Reset()
	code := runMain(d, []string{CmdRun}, nil,
		strings.NewReader(fmt.Sprintf(probe, `["--bogus=1"]`)), &out, io.Discard)
	if code != 1 || !strings.Contains(out.String(), `"code":"fixable"`) ||
		!strings.Contains(out.String(), "--bogus=1") {
		t.Errorf("probe with an undeclared flag: exit %d, %q", code, out.String())
	}
	out.Reset()
	bad := strings.Replace(fmt.Sprintf(probe, `[]`), `"probe":true`,
		`"probe":true,"build_id":"k3v9x2ab"`, 1)
	if code := runMain(d, []string{CmdRun}, nil, strings.NewReader(bad), &out,
		io.Discard); code != 2 {
		t.Errorf("probe with a build ID: exit %d, %q", code, out.String())
	}
}

// strayDriver prints to its stdout, and starts a child that prints to the
// stdout it inherits, during run.
func strayDriver() *Driver {
	return &Driver{Describe: Describe{Name: "stray", Version: "0",
		Capabilities: Capabilities{Keep: "none", Containerfile: "ignore",
			GuestOS: []string{"linux"}}},
		Run: func(ctx context.Context, _ *driver.Start, host driver.Host) ([]string,
			error) {
			fmt.Println("stray line")
			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(), testDriverEnv+"=child")
			cmd.Stdout = os.Stdout
			if err := cmd.Run(); err != nil {
				return nil, err
			}
			host.Wait(ctx)
			return nil, nil
		},
		Reap: func(context.Context, *ReapRequest) (int, error) { return 0, nil }}
}

// DRV-16, DRV-39: during run nothing but messages reaches stdout; the
// driver's and its child's prints go to stderr.
func TestRunRedirectsStdout(t *testing.T) {
	cmd := exec.Command(os.Args[0], CmdRun)
	cmd.Env = append(os.Environ(), testDriverEnv+"=stray")
	cmd.Stdin = strings.NewReader(startLine)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v; stderr %s", err, stderr.String())
	}
	if stdout.String() != `{"type":"stopped"}`+"\n" {
		t.Errorf("stdout %q, want only the stopped message", stdout.String())
	}
	for _, want := range []string{"stray line", "child stray line"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr %q lacks %q", stderr.String(), want)
		}
	}
}

// teardownDriver reports its relay, then, once its input has ended, writes
// to its closed stdout and stderr many times before it marks its teardown
// done in the file DRIVERCMD_TEST_MARK names.
func teardownDriver() *Driver {
	return &Driver{Describe: Describe{Name: "teardown", Version: "0",
		Capabilities: Capabilities{Keep: "none", Containerfile: "ignore",
			GuestOS: []string{"linux"}}},
		Run: func(ctx context.Context, _ *driver.Start, host driver.Host) ([]string,
			error) {
			_ = host.ClaimRelay(ctx, driver.RelayStarted{GuestOS: "linux",
				Claim: driver.ClaimEndpoint{Network: "tcp",
					Address: "127.0.0.1:1"}})
			// Wait returns when stdin ends, which the test closes only
			// after stdout and stderr, so both are closed by now.
			host.Wait(ctx)
			for range 100 {
				fmt.Fprintln(os.Stderr, "tearing down")
				host.Exited(driver.ExitedBuild, "tearing down")
			}
			return nil, os.WriteFile(os.Getenv("DRIVERCMD_TEST_MARK"), []byte("done"),
				0o600)
		},
		Reap: func(context.Context, *ReapRequest) (int, error) { return 0, nil }}
}

// DRV-51: with stdin, stdout and stderr all closed, as a killed orchestrator
// leaves them, the driver still completes its teardown.
func TestRunTeardownWithClosedOutput(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "mark")
	cmd := exec.Command(os.Args[0], CmdRun)
	cmd.Env = append(os.Environ(), testDriverEnv+"=teardown", "DRIVERCMD_TEST_MARK="+mark)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(stdin, startLine)
	if got := readType(t, bufio.NewReader(stdout)); got != "relay-started" {
		t.Fatalf("first message %s", got)
	}
	// Close the driver's output first and its input last, so the
	// teardown it starts when stdin ends always writes to closed pipes.
	_ = stdout.Close()
	_ = stderr.Close()
	_ = stdin.Close()
	_ = cmd.Wait()
	if data, err := os.ReadFile(mark); err != nil || string(data) != "done" {
		t.Errorf("teardown did not complete: %q, %v", data, err)
	}
}

// DRV-61: the state file is written atomically, read back, absent when not
// written, and refused over 64 KiB.
func TestStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build-kit-k3v9x2ab.state")
	var got struct{ Runtime string }
	if found, err := LoadState(path, &got); found || err != nil {
		t.Fatalf("LoadState of nothing = %v, %v", found, err)
	}
	if err := SaveState(path, struct{ Runtime string }{"podman"}); err != nil {
		t.Fatal(err)
	}
	if found, err := LoadState(path, &got); !found || err != nil || got.Runtime != "podman" {
		t.Fatalf("LoadState = %v, %v, %+v", found, err, got)
	}
	if err := SaveState(path, strings.Repeat("x", MaxStateFile)); err == nil {
		t.Error("SaveState wrote more than 64 KiB")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files, want the state file only", len(entries))
	}
}
