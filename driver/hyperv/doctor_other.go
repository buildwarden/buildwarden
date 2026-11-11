//go:build !windows

package hyperv

// probeDoctorNet is unreachable off Windows: Doctor reports the driver
// unavailable before it probes the network.
func probeDoctorNet(Capabilities) doctorNet {
	return doctorNet{Err: errUnsupported()}
}
