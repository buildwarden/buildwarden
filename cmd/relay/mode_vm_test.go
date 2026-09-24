package main

import "testing"

// When the sink env vars are set (a driver with no host-shared filesystem, e.g.
// Hyper-V), vmConfig must route them into the relay config so relay.Start picks
// the httpSink.
func TestVMConfig_SinkFromEnv(t *testing.T) {
	t.Setenv("OUTPUT_SINK_URL", "http://192.168.240.1:8299")
	t.Setenv("OUTPUT_SINK_TOKEN", "tok-123")

	cfg := vmConfig("/ledger", "/context", "/sig", "bodies", "build.sh")

	if cfg.OutputSinkURL != "http://192.168.240.1:8299" {
		t.Errorf("OutputSinkURL = %q, want the env value", cfg.OutputSinkURL)
	}
	if cfg.OutputSinkToken != "tok-123" {
		t.Errorf("OutputSinkToken = %q, want tok-123", cfg.OutputSinkToken)
	}
	if cfg.LedgerDir != "/ledger" || cfg.ContextDir != "/context" ||
		cfg.SignalDir != "/sig" || cfg.CaptureMode != "bodies" {
		t.Errorf("base fields not mapped through: %+v", cfg)
	}
	if cfg.BuildScriptPath != "build.sh" {
		t.Errorf("BuildScriptPath = %q, want build.sh", cfg.BuildScriptPath)
	}
}

// With no sink env set (qemu/vz, which share a host path), vmConfig must leave
// the sink fields empty so relay.Start selects the local filesystem sink.
func TestVMConfig_LocalSinkByDefault(t *testing.T) {
	t.Setenv("OUTPUT_SINK_URL", "")
	t.Setenv("OUTPUT_SINK_TOKEN", "")

	cfg := vmConfig("/ledger", "/context", "", "", "")

	if cfg.OutputSinkURL != "" || cfg.OutputSinkToken != "" {
		t.Errorf("expected empty sink config (local sink), got url=%q token=%q",
			cfg.OutputSinkURL, cfg.OutputSinkToken)
	}
}
