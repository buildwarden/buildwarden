package driver

import "fmt"

// DefaultExtensions returns the standard set of extensions applied to all builds.
func DefaultExtensions() []Extension {
	return []Extension{
		&ExtTrustStore{},
		&ExtBazel{},
		&ExtCACerts{},
		&ExtEpoch{},
	}
}

// RunExtensions executes BeforeBuild on all extensions and collects env vars.
func RunExtensions(exts []Extension, ctx *ExtensionContext) (map[string]string, error) {
	env := make(map[string]string)
	for _, ext := range exts {
		if err := ext.BeforeBuild(ctx); err != nil {
			return nil, err
		}
		for k, v := range ext.Env() {
			if _, exists := env[k]; exists {
				return nil, fmt.Errorf("env collision: %s", k)
			}
			env[k] = v
		}
	}
	return env, nil
}
