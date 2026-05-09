package orchestrator

type Extension interface {
	BeforeBuild(env *CtrEnv) error
	Env() map[string]string
}
