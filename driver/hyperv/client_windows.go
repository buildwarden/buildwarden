//go:build windows

package hyperv

import (
	"context"
	"fmt"
	"time"
)

// clientProvisioner delegates ONLY the host network-standup operations to the
// privileged service over the pipe; every VM-lifecycle operation runs
// in-process via the embedded localProvisioner (the Hyper-V Administrators group
// covers those with no elevation, so there is no reason to marshal them). This
// is the "delegate only network standup" scope: the service's attack surface
// stays limited to New-VMSwitch / New-NetNat, and VM ops keep their direct path.
type clientProvisioner struct {
	*localProvisioner
}

// newClientProvisioner returns a Provisioner whose network standup is delegated
// to the privileged service.
func newClientProvisioner(verbose bool) Provisioner {
	return &clientProvisioner{localProvisioner: &localProvisioner{verbose: verbose}}
}

func (c *clientProvisioner) EnsureNetwork(_ context.Context, spec NetworkSpec) (*NetworkResources, error) {
	conn, err := dialServicePipe(5 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial privileged service (is it installed? `warden hyperv service install`): %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	resp, err := clientRoundTrip(conn, svcRequest{Op: opEnsureNetwork, Spec: &spec})
	if err != nil {
		return nil, err
	}
	return resp.Resources, nil
}

func (c *clientProvisioner) TeardownNetwork(_ context.Context, id string) error {
	conn, err := dialServicePipe(5 * time.Second)
	if err != nil {
		return fmt.Errorf("dial privileged service: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	_, err = clientRoundTrip(conn, svcRequest{Op: opTeardownNetwork, ID: id})
	return err
}
