//go:build windows

package hyperv

import "context"

// probeDoctorNet reads, without changing anything, the network a build on
// this host would use: the NAT and the durable network's slot switches, the
// VMs attached to each slot, the relay VMs' Notes, and the host's resolver
// source.
func probeDoctorNet(c Capabilities) doctorNet {
	dn := doctorNet{DNS: defaultRouteDNS}
	out, err := runPS(netProbeScript(c.DevSwitch, DefaultNATPrefix))
	if err != nil {
		dn.Err = err
		return dn
	}
	dn.ReadBack = parseNetReadback(out)
	p := &localProvisioner{}
	ctx := context.Background()
	if dn.Attached, err = p.SwitchAdapters(ctx, dn.ReadBack.slotNames(c.DevSwitch)); err != nil {
		dn.Err = err
		return dn
	}
	if dn.Notes, err = p.RelayNotes(ctx); err != nil {
		dn.Err = err
	}
	return dn
}
