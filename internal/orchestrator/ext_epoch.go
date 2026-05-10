package orchestrator

type ExtEpoch struct{}

func (e *ExtEpoch) BeforeBuild(env *CtrEnv) error { return nil }

func (e *ExtEpoch) Env() map[string]string {
	return map[string]string{"SOURCE_DATE_EPOCH": "0"}
}
