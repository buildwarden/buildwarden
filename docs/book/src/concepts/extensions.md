# Extensions

Extensions configure the build environment to work correctly through the relay's MITM proxy. They inject certificates, environment variables, and setup scripts into the build container.

## How Extensions Work

Each extension implements two methods:

- `BeforeBuild(env)` — Runs after the relay starts (has access to the ephemeral CA cert). Writes files into `.warden/ext.d/`.
- `Env()` — Returns environment variables to inject into the Dockerfile after each `FROM` directive.

The orchestrator injects this into the rewritten Dockerfile after every `FROM`:

```dockerfile
COPY .warden /.warden
RUN find /.warden/ext.d/ -exec sh {} \;
```

## Built-in Extensions

### TrustStore

Installs the relay's ephemeral CA certificate into the system trust store. This enables `apt`, `curl`, and other tools to trust the relay's MITM certificates.

Writes `/.warden/ca.crt` and configures `/etc/ssl/certs/`.

### Pip

Sets `PIP_CERT=/etc/ssl/certs/warden.crt` so pip trusts the relay for HTTPS downloads from PyPI.

### Bazel

Configures Bazel to use the relay's certificate for remote fetches.

### Epoch

Sets `SOURCE_DATE_EPOCH=0` for reproducible builds. Ensures timestamps in build outputs are deterministic.

## Extension Order

Extensions run in this order:
1. TrustStore (must be first — other extensions depend on TLS working)
2. Pip
3. Bazel
4. Epoch

## Adding Custom Extensions

Extensions are Go types in `internal/orchestrator/`. To add one:

1. Create `ext_myext.go` implementing the `Extension` interface
2. Add it to the `exts` slice in `orchestrator.go`

```go
type ExtMyTool struct{}

func (e *ExtMyTool) BeforeBuild(env *CtrEnv) error {
    // Write setup scripts to env.wardenScriptPath()
    return nil
}

func (e *ExtMyTool) Env() map[string]string {
    return map[string]string{
        "MY_CERT_PATH": "/etc/ssl/certs/warden.crt",
    }
}
```
