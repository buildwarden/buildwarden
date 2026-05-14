package driver

// ExtensionContext provides driver-agnostic access to the build environment
// for extensions that inject CA certs, env vars, and configuration.
type ExtensionContext struct {
	WardenDir string
	ScriptDir string
	LedgerDir string
	RelayIP   string
	BuildID   string
}

// Extension modifies the build environment before execution.
type Extension interface {
	BeforeBuild(ctx *ExtensionContext) error
	Env() map[string]string
}
