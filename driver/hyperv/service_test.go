package hyperv

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// netFake is a minimal Provisioner double for the pipe-protocol tests. It
// records the network-standup calls and returns canned results; the VM-lifecycle
// methods are never exercised over the pipe (network standup is the only
// delegated tier).
type netFake struct {
	ensureSpec   NetworkSpec
	ensureCalled bool
	teardownID   string
	teardownErr  error
	ensureErr    error
}

func (f *netFake) EnsureNetwork(_ context.Context, spec NetworkSpec) (*NetworkResources, error) {
	f.ensureCalled = true
	f.ensureSpec = spec
	if f.ensureErr != nil {
		return nil, f.ensureErr
	}
	return &NetworkResources{
		BuildSwitch: spec.SwitchName,
		NATName:     spec.SwitchName + "-nat",
	}, nil
}
func (f *netFake) TeardownNetwork(_ context.Context, id string) error {
	f.teardownID = id
	return f.teardownErr
}
func (f *netFake) CreateVM(context.Context, VMSpec) (*VMHandle, error) { panic("not over pipe") }
func (f *netFake) CreateRelayVM(context.Context, RelayVMSpec) (*VMHandle, error) {
	panic("not over pipe")
}
func (f *netFake) StartVM(context.Context, string) error                { panic("not over pipe") }
func (f *netFake) StopVM(context.Context, string) error                 { panic("not over pipe") }
func (f *netFake) RemoveVM(context.Context, string) error               { panic("not over pipe") }
func (f *netFake) CreateDiffDisk(context.Context, string, string) error { panic("not over pipe") }
func (f *netFake) VMState(context.Context, string) (string, error)      { panic("not over pipe") }
func (f *netFake) GuestKVP(context.Context, string) (map[string]string, error) {
	panic("not over pipe")
}
func (f *netFake) DisksInUse(context.Context) ([]string, error) { panic("not over pipe") }
func (f *netFake) ListVMs(context.Context, string) ([]string, error) {
	panic("not over pipe")
}
func (f *netFake) RelayNotes(context.Context) (map[string]string, error) {
	panic("not over pipe")
}
func (f *netFake) SwitchAdapters(context.Context, []string) (map[string][]string, error) {
	panic("not over pipe")
}

// roundTripOverPipe runs serveConn on one end of a net.Pipe and clientRoundTrip
// on the other, returning the client's result.
func roundTripOverPipe(t *testing.T, prov Provisioner, req svcRequest) (svcResponse, error) {
	t.Helper()
	srv, cli := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer srv.Close()
		_ = serveConn(context.Background(), srv, prov)
	}()
	_ = cli.SetDeadline(time.Now().Add(5 * time.Second))
	resp, err := clientRoundTrip(cli, req)
	cli.Close()
	<-done
	return resp, err
}

func TestService_EnsureNetworkRoundTrip(t *testing.T) {
	fake := &netFake{}
	spec := NetworkSpec{
		ID:         "abc",
		SwitchName: "warden-abc",
		SwitchType: "Private",
		NATPrefix:  "192.168.240.0/20",
		HostIP:     "192.168.240.1",
	}
	resp, err := roundTripOverPipe(t, fake, svcRequest{Op: opEnsureNetwork, Spec: &spec})
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if !fake.ensureCalled {
		t.Fatal("server did not invoke EnsureNetwork")
	}
	if fake.ensureSpec.SwitchName != "warden-abc" {
		t.Errorf("server got switch %q, want warden-abc", fake.ensureSpec.SwitchName)
	}
	if resp.Resources == nil || resp.Resources.BuildSwitch != "warden-abc" {
		t.Errorf("client got resources %+v, want BuildSwitch warden-abc", resp.Resources)
	}
}

func TestService_TeardownRoundTrip(t *testing.T) {
	fake := &netFake{}
	if _, err := roundTripOverPipe(
		t, fake, svcRequest{Op: opTeardownNetwork, ID: "warden-abc"},
	); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if fake.teardownID != "warden-abc" {
		t.Errorf("server got teardown id %q, want warden-abc", fake.teardownID)
	}
}

func TestService_Ping(t *testing.T) {
	resp, err := roundTripOverPipe(t, &netFake{}, svcRequest{Op: opPing})
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if !resp.OK {
		t.Error("ping response not OK")
	}
}

func TestService_VersionMismatchRejected(t *testing.T) {
	// handleServiceRequest is pure; hit it directly with a wrong version.
	resp := handleServiceRequest(
		context.Background(), &netFake{}, svcRequest{Version: 999, Op: opPing},
	)
	if resp.Err == "" {
		t.Fatal("expected a version-mismatch error")
	}
}

func TestService_UnsupportedOpRejected(t *testing.T) {
	fake := &netFake{}
	_, err := roundTripOverPipe(t, fake, svcRequest{Op: "create_vm"})
	if err == nil {
		t.Fatal("expected an error for a non-network op")
	}
}

func TestService_EnsureNetworkNilSpec(t *testing.T) {
	resp := handleServiceRequest(
		context.Background(), &netFake{},
		svcRequest{Version: serviceProtocolVersion, Op: opEnsureNetwork},
	)
	if resp.Err == "" {
		t.Fatal("expected an error for a nil spec")
	}
}

// protocol1Service answers one request on conn as a service installed by a
// warden at protocol 1 does: the version check fails before the op is read.
func protocol1Service(conn net.Conn) {
	defer conn.Close()
	var req svcRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	_ = json.NewEncoder(conn).Encode(svcResponse{
		Version: 1,
		Err: fmt.Sprintf("protocol version mismatch: service speaks 1, "+
			"client sent %d; reinstall the service (warden hyperv service install)",
			req.Version),
	})
}

// pingOver runs serve on one end of a net.Pipe and pingService on the other.
func pingOver(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	srv, cli := net.Pipe()
	go serve(srv)
	defer cli.Close()
	_ = cli.SetDeadline(time.Now().Add(5 * time.Second))
	return pingService(cli)
}

func TestPingService(t *testing.T) {
	tests := []struct {
		name  string
		serve func(net.Conn)
		want  string // substring of the stale reason; "" for a usable service
	}{
		{"this protocol is usable", func(c net.Conn) {
			defer c.Close()
			_ = serveConn(context.Background(), c, &netFake{})
		}, ""},
		{"a protocol-1 service is stale, naming both versions",
			protocol1Service, "speaks protocol 1, this warden 2"},
		{"an unreadable reply is stale, not absent", func(c net.Conn) {
			defer c.Close()
			_ = json.NewDecoder(c).Decode(&svcRequest{})
			_, _ = c.Write([]byte("not json\n"))
		}, "its ping fails: decode response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pingOver(t, tt.serve)
			switch {
			case tt.want == "" && got != "":
				t.Fatalf("pingService = %q, want usable", got)
			case !strings.Contains(got, tt.want):
				t.Fatalf("pingService = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}
