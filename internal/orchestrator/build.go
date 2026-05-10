package orchestrator

type BuildEnv interface {
	Build(config *BuildConfig) error
	Shell(config *BuildConfig) error
}

type BuildConfig struct {
	Context       string
	Containerfile string
	Capture       string
	OutputDir     string
	Compress      bool
	RelayImage    string
}
