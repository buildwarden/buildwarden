package hyperv

import (
	"reflect"
	"testing"
)

// TestParseRelayNotes reads RelayNotes' "<vm>\t<notes>" lines as the
// PowerShell host prints them.
func TestParseRelayNotes(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want map[string]string
	}{
		{"empty output", "", map[string]string{}},
		{"blank lines only", "\r\n\n  \n", map[string]string{}},
		{"one relay", "warden-relay-a\twarden-relay-ip=10.200.0.2\n",
			map[string]string{"warden-relay-a": "warden-relay-ip=10.200.0.2"}},
		{"several relays, CRLF", "warden-relay-a\tnote a\r\nwarden-relay-b\tnote b\r\n",
			map[string]string{"warden-relay-a": "note a", "warden-relay-b": "note b"}},
		{"no trailing newline", "warden-relay-a\tnote a",
			map[string]string{"warden-relay-a": "note a"}},
		{"empty notes", "warden-relay-a\t\n",
			map[string]string{"warden-relay-a": ""}},
		{"no tab is a VM with no notes", "warden-relay-a\n",
			map[string]string{"warden-relay-a": ""}},
		{"tab inside notes kept", "warden-relay-a\tleft\tright\n",
			map[string]string{"warden-relay-a": "left\tright"}},
		{"padding trimmed", "  warden-relay-a \t note a  \n",
			map[string]string{"warden-relay-a": "note a"}},
		{"empty name skipped", "\torphan notes\nwarden-relay-a\tnote a\n",
			map[string]string{"warden-relay-a": "note a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRelayNotes(tt.out); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseRelayNotes(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
	}
}

// TestParseSwitchAdapters reads SwitchAdapters' "<switch>\t<vm>" lines: a VM
// once per adapter, in output order.
func TestParseSwitchAdapters(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want map[string][]string
	}{
		{"empty output", "", map[string][]string{}},
		{"blank lines only", "\r\n\n  \n", map[string][]string{}},
		{"one adapter", "warden-dev-1\twarden-relay-a\n",
			map[string][]string{"warden-dev-1": {"warden-relay-a"}}},
		{"several switches and VMs, CRLF",
			"warden-dev-1\twarden-relay-a\r\nwarden-dev-1\twarden-build-a\r\n" +
				"warden-dev-2\twarden-relay-b\r\n",
			map[string][]string{
				"warden-dev-1": {"warden-relay-a", "warden-build-a"},
				"warden-dev-2": {"warden-relay-b"},
			}},
		{"a VM once per adapter", "warden-dev-1\tvm\nwarden-dev-1\tvm\n",
			map[string][]string{"warden-dev-1": {"vm", "vm"}}},
		{"no trailing newline", "warden-dev-1\twarden-relay-a",
			map[string][]string{"warden-dev-1": {"warden-relay-a"}}},
		{"padding trimmed", " warden-dev-1 \t warden-relay-a \n",
			map[string][]string{"warden-dev-1": {"warden-relay-a"}}},
		// An adapter with no VM name is still an adapter on the switch, so it
		// stays in the list for the slot check to refuse.
		{"adapter with no VM name kept", "warden-dev-1\t\n",
			map[string][]string{"warden-dev-1": {""}}},
		{"line with no tab skipped", "garbage\nwarden-dev-1\twarden-relay-a\n",
			map[string][]string{"warden-dev-1": {"warden-relay-a"}}},
		{"empty switch skipped", "\twarden-relay-a\n", map[string][]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSwitchAdapters(tt.out); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSwitchAdapters(%q) = %q, want %q",
					tt.out, got, tt.want)
			}
		})
	}
}
