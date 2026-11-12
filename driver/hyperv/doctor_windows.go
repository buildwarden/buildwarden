//go:build windows

package hyperv

import "context"

// probeDoctorNet reads, without changing anything, the network a build on
// this host would use: the NAT and the durable network's slot switches, the
// VMs attached to each slot, the relay VMs' Notes, and the host's resolver
// source.
func probeDoctorNet(c Capabilities) doctorNet {
	dn := doctorNet{DNS: defaultRouteDNS}
	other := DefaultNATPrefix
	if c.NetworkStrategy() == NetReuseDurable {
		other = "" // a durable build reads only <network>-nat
	}
	out, err := runPS(netProbeScript(c.DevSwitch, other))
	if err != nil {
		dn.Err = err
		return dn
	}
	dn.ReadBack = parseNetReadback(out)
	p := &localProvisioner{}
	ctx := context.Background()
	slots := dn.ReadBack.slotNames(c.DevSwitch)
	if dn.Attached, err = p.SwitchAdapters(ctx, slots); err != nil {
		dn.Err = err
		return dn
	}
	if dn.Notes, err = p.RelayNotes(ctx); err != nil {
		dn.Err = err
	}
	return dn
}
