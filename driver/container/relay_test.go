package container

import "testing"

func TestParsePublishedAddr(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:49153\n":              "127.0.0.1:49153",
		"[::1]:49154\n127.0.0.1:49153\n": "127.0.0.1:49153",
		"0.0.0.0:32768\n":                "127.0.0.1:32768",
		"  127.0.0.1:5000  ":             "127.0.0.1:5000",
	} {
		got, err := parsePublishedAddr(in)
		if err != nil || got != want {
			t.Errorf("parsePublishedAddr(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := parsePublishedAddr("[::1]:1\n"); err == nil {
		t.Error("IPv6-only mapping accepted")
	}
}
