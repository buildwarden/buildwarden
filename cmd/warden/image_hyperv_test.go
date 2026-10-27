package main

import (
	"testing"

	"github.com/buildwarden/buildwarden/driver/hyperv"
)

func TestHypervImageSource(t *testing.T) {
	const fwlink = "https://go.microsoft.com/fwlink/?linkid=2293215"
	downloaded := hyperv.ImageStatus{Base: `C:\cache\26100.vhdx`, BaseURL: fwlink}
	local := hyperv.ImageStatus{Base: `D:\images\server.vhdx`}
	cases := []struct {
		name                   string
		source, conf, update   string
		st                     hyperv.ImageStatus
		wantSrc                string
		wantFetch, wantTolerat bool
	}{
		{name: "first prepare uses the default", st: hyperv.ImageStatus{},
			wantSrc: "", wantFetch: true},
		{name: "first prepare uses the config", conf: `E:\x.vhdx`,
			wantSrc: `E:\x.vhdx`, wantFetch: true},
		{name: "explicit source wins and is strict", source: "https://x/y.vhdx",
			conf: `E:\x.vhdx`, st: downloaded,
			wantSrc: "https://x/y.vhdx", wantFetch: true},
		{name: "refresh re-checks a downloaded base", st: downloaded,
			wantSrc: fwlink, wantFetch: true, wantTolerat: true},
		{name: "reuse keeps a downloaded base", st: downloaded, update: "reuse"},
		{name: "a local base is never replaced", st: local},
		{name: "refresh follows a changed config", conf: "https://new/z.vhdx", st: local,
			wantSrc: "https://new/z.vhdx", wantFetch: true, wantTolerat: true},
		{name: "reuse with the config already active", conf: fwlink, update: "reuse",
			st: downloaded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.VM.WindowsImage, cfg.VM.WindowsImageUpdate = c.conf, c.update
			src, fetch, tolerant := hypervImageSource(c.source, cfg, c.st)
			if src != c.wantSrc || fetch != c.wantFetch || tolerant != c.wantTolerat {
				t.Errorf("got (%q, fetch=%v, tolerant=%v), want (%q, %v, %v)",
					src, fetch, tolerant,
					c.wantSrc, c.wantFetch, c.wantTolerat)
			}
		})
	}
}
