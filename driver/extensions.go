package driver

// DefaultExtensions returns the standard set of extensions applied to all builds.
func DefaultExtensions() []Extension {
	return []Extension{
		&ExtTrustStore{},
		&ExtBazel{},
		&ExtCACerts{},
		&ExtEpoch{},
	}
}
