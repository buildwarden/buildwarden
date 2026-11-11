//go:build windows

package hyperv

import (
	"errors"
	"net/netip"
)

// defaultRouteDNS is the host's resolver source on Windows (plan task 30).
func defaultRouteDNS() ([]netip.Addr, string, error) {
	return nil, "", errors.New("the Windows default-route DNS source lands in plan task 30")
}
