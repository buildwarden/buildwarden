package main

type ExtPip struct{}

func (e *ExtPip) BeforeBuild(env *CtrEnv) error {
	return nil
}

func (e *ExtPip) Env() map[string]string {
	return map[string]string{
		"PIP_CERT":      "/etc/ssl/certs/warden.crt",
		"UV_NATIVE_TLS": "1",
	}
}
