// Package resolvers is the VM drivers' one selection of the relay's upstream
// resolvers, which they send in the claim as upstream_dns. A driver gives it
// the list and its own reachability rule: Check takes [vm] dns, which the user
// named, and refuses its first unusable entry; Select takes the host's
// resolvers, which are only the default's source, and keeps those the rule
// allows. The host sources for Linux and macOS read /etc/resolv.conf through
// an injected file read; Hyper-V's Windows source stays in its driver.
//
// resolvers imports only buildlink and the standard library; its errors are
// plain for each driver to wrap in driver.Fixable with its own fix.
package resolvers

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/buildwarden/buildwarden/driver/buildlink"
)

// VMDNS is the source label of a list taken from [vm] dns.
const VMDNS = "[vm] dns"

// Rule is a driver's reachability rule: why its relay cannot use addr, or ""
// when it can.
type Rule func(addr netip.Addr) string

// Rules is the rule that gives the first reason any of rules gives.
func Rules(rules ...Rule) Rule {
	return func(addr netip.Addr) string {
		for _, r := range rules {
			if why := r(addr); why != "" {
				return why
			}
		}
		return ""
	}
}

// Loopback excludes a loopback address.
func Loopback(addr netip.Addr) string {
	if addr.IsLoopback() {
		return "loopback"
	}
	return ""
}

// LinkLocal excludes a link-local address.
func LinkLocal(addr netip.Addr) string {
	if addr.IsLinkLocalUnicast() {
		return "link-local"
	}
	return ""
}

// InPrefix excludes an address inside p, naming it as what.
func InPrefix(p netip.Prefix, what string) Rule {
	return func(addr netip.Addr) string {
		if p.Contains(addr.Unmap()) {
			return "inside " + what + " " + p.String()
		}
		return ""
	}
}

// BuildLink excludes an address inside the build link, which the relay
// routes to the build VM.
func BuildLink(link buildlink.Link) Rule {
	return InPrefix(link.Net, "the build link")
}

// InLink reports whether addr is inside the build link.
func InLink(link buildlink.Link, addr netip.Addr) bool {
	return link.Net.Contains(addr.Unmap())
}

// Excluded is a host resolver a rule dropped, and why.
type Excluded struct {
	Addr   netip.Addr
	Reason string
}

// String is the address and its reason, e.g. "127.0.0.53 (loopback)".
func (e Excluded) String() string { return e.Addr.String() + " (" + e.Reason + ")" }

// Select splits the host's resolvers into those the rule allows, in order,
// and those it excludes, with why.
func Select(host []netip.Addr, rule Rule) (use []netip.Addr, excluded []Excluded) {
	for _, a := range host {
		if why := rule(a); why != "" {
			excluded = append(excluded, Excluded{Addr: a, Reason: why})
			continue
		}
		use = append(use, a)
	}
	return use, excluded
}

// UnusableError is Check's refusal: the [vm] dns entry the rule refuses, and
// why.
type UnusableError struct {
	Addr   netip.Addr
	Reason string
}

func (e *UnusableError) Error() string {
	return fmt.Sprintf("%s lists %s, which the relay cannot use (%s)", VMDNS, e.Addr,
		e.Reason)
}

// Check returns vmDNS unchanged, in order, when the rule allows every entry,
// and otherwise an *UnusableError naming the first entry it refuses: the user
// named each entry, so none is dropped.
func Check(vmDNS []netip.Addr, rule Rule) ([]netip.Addr, error) {
	for _, a := range vmDNS {
		if why := rule(a); why != "" {
			return nil, &UnusableError{Addr: a, Reason: why}
		}
	}
	return vmDNS, nil
}

// Join is the list as doctor and the refusals print it, e.g.
// "10.9.0.2, 10.9.0.3".
func Join(list []netip.Addr) string {
	s := make([]string, len(list))
	for i, a := range list {
		s[i] = a.String()
	}
	return strings.Join(s, ", ")
}

// JoinExcluded is the excluded resolvers as the refusals print them.
func JoinExcluded(excluded []Excluded) string {
	s := make([]string, len(excluded))
	for i, e := range excluded {
		s[i] = e.String()
	}
	return strings.Join(s, ", ")
}
