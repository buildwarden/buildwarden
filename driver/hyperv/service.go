package hyperv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Phase 4 privileged-service wire protocol.
//
// The Hyper-V driver splits privileged work into two tiers (see capability.go):
// VM lifecycle needs only the Hyper-V Administrators group (runs in-process),
// but host network standup (New-VMSwitch / New-NetNat / New-NetIPAddress) needs
// full Administrator. The privileged service is a LocalSystem process that hosts
// a localProvisioner behind a named pipe so a NON-elevated build can delegate
// ONLY the network-standup operations to it; every VM op still runs in-process.
//
// The pipe carries a small TYPED op set — never raw PowerShell or arbitrary
// argument strings — so a caller that clears the pipe ACL still cannot ask the
// service to run anything but these three structured operations.

const (
	// ServicePipeName is the well-known pipe the privileged service listens on.
	// The clientProvisioner dials it to delegate network standup, and capability
	// detection dials it (ping) to probe reachability.
	ServicePipeName = `\\.\pipe\warden-hyperv-svc`

	// serviceProtocolVersion is bumped on any wire-incompatible change. Both
	// ends check it on every request, so a stale service running against a newer
	// warden fails fast with a reinstall message instead of misbehaving.
	serviceProtocolVersion = 1
)

// svcOp is the typed operation set the pipe accepts. Only network standup is
// delegated; VM lifecycle is never sent over the pipe.
type svcOp string

const (
	opPing            svcOp = "ping"             // reachability + version handshake
	opEnsureNetwork   svcOp = "ensure_network"   // EnsureNetwork(spec)
	opTeardownNetwork svcOp = "teardown_network" // TeardownNetwork(id)
)

// svcRequest is one structured request. Exactly the fields for one op are set.
type svcRequest struct {
	Version int          `json:"version"`
	Op      svcOp        `json:"op"`
	Spec    *NetworkSpec `json:"spec,omitempty"` // ensure_network
	ID      string       `json:"id,omitempty"`   // teardown_network
}

// svcResponse is the single reply to a request.
type svcResponse struct {
	Version   int               `json:"version"`
	OK        bool              `json:"ok"`
	Err       string            `json:"err,omitempty"`
	Resources *NetworkResources `json:"resources,omitempty"` // ensure_network result
}

// handleServiceRequest dispatches one request against prov and returns the
// response. It is pure (no I/O), so the whole protocol is unit-testable with an
// in-memory Provisioner and no pipe. Only the two network-standup ops touch the
// provisioner; anything else is rejected.
func handleServiceRequest(ctx context.Context, prov Provisioner, req svcRequest) svcResponse {
	resp := svcResponse{Version: serviceProtocolVersion}
	if req.Version != serviceProtocolVersion {
		resp.Err = fmt.Sprintf(
			"protocol version mismatch: service speaks %d, client sent %d; reinstall the service (warden hyperv service install)",
			serviceProtocolVersion, req.Version)
		return resp
	}
	switch req.Op {
	case opPing:
		resp.OK = true
	case opEnsureNetwork:
		if req.Spec == nil {
			resp.Err = "ensure_network: nil spec"
			return resp
		}
		res, err := prov.EnsureNetwork(ctx, *req.Spec)
		if err != nil {
			resp.Err = err.Error()
			return resp
		}
		resp.OK = true
		resp.Resources = res
	case opTeardownNetwork:
		if err := prov.TeardownNetwork(ctx, req.ID); err != nil {
			resp.Err = err.Error()
			return resp
		}
		resp.OK = true
	default:
		resp.Err = fmt.Sprintf("unsupported op %q (the pipe delegates network standup only)", req.Op)
	}
	return resp
}

// serveConn handles exactly one request/response on a connection, then returns
// (the caller closes the connection). Network standup is infrequent, so one op
// per connection keeps the server trivially correct.
func serveConn(ctx context.Context, rw io.ReadWriter, prov Provisioner) error {
	var req svcRequest
	if err := json.NewDecoder(rw).Decode(&req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	resp := handleServiceRequest(ctx, prov, req)
	if err := json.NewEncoder(rw).Encode(resp); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// clientRoundTrip writes one request and reads one response over rw. It is the
// transport half the clientProvisioner and the ping probe share.
func clientRoundTrip(rw io.ReadWriter, req svcRequest) (svcResponse, error) {
	req.Version = serviceProtocolVersion
	if err := json.NewEncoder(rw).Encode(req); err != nil {
		return svcResponse{}, fmt.Errorf("encode request: %w", err)
	}
	var resp svcResponse
	if err := json.NewDecoder(rw).Decode(&resp); err != nil {
		return svcResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if resp.Err != "" {
		return resp, fmt.Errorf("service: %s", resp.Err)
	}
	return resp, nil
}
