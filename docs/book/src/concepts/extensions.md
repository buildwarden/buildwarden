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

### TrustStore (`ext_truststore.go`)

Installs the relay's ephemeral CA certificate into the system trust store. This enables `apt`, `apk`, `curl`, and other tools that use the system CA store to trust the relay automatically.

Writes `/.warden/ca.crt` and configures `/etc/ssl/certs/`.

### CA Cert Environment Variables (`ext_cacerts_env.go`)

Sets environment variables for package managers that don't use the system CA store by default:

| Env Var | Package Managers |
|---------|-----------------|
| `PIP_CERT` | pip |
| `UV_NATIVE_TLS` | uv |
| `REQUESTS_CA_BUNDLE` | poetry, conda, conan |
| `NODE_EXTRA_CA_CERTS` | npm, yarn, pnpm, bun |
| `SSL_CERT_FILE` | Ruby gem/bundler, Erlang/Elixir, general OpenSSL consumers |
| `CURL_CA_BUNDLE` | PHP composer, libcurl-based tools |
| `NIX_SSL_CERT_FILE` | nix |
| `HEX_CACERTS_PATH` | Elixir hex |

All values are harmless when the corresponding tool is not installed.

### JKS TrustStore (`ext_jks_truststore.go`)

Creates a Java KeyStore (JKS) at `/.warden/certs.jks` containing the relay CA, and sets JVM trust flags via:

| Env Var | JVM Tools |
|---------|-----------|
| `MAVEN_OPTS` | Maven |
| `GRADLE_OPTS` | Gradle |

Also writes `/.warden/bazel.bazelrc` to `/etc/bazel.bazelrc` for Bazel's host JVM.

### Epoch (`ext_epoch.go`)

Sets `SOURCE_DATE_EPOCH=0` for reproducible builds. Ensures timestamps in build outputs are deterministic.

## Extension Order

Extensions run in this order:
1. TrustStore (must be first — other extensions depend on TLS working)
2. JKS TrustStore (creates JKS from CA cert)
3. CA Certs Env (env vars only, no file I/O)
4. Epoch

## Package Manager Compatibility

Tested and verified working through warden:

| Manager | Supported By |
|---------|-------------|
| apt, apk, dnf | TrustStore (system CA) |
| go modules | TrustStore (system CA) |
| cargo | TrustStore (system CA) |
| nuget/.NET | TrustStore (system CA) |
| gem/bundler | CA Certs Env (`SSL_CERT_FILE`) |
| composer | CA Certs Env (`CURL_CA_BUNDLE`) |
| npm, yarn, pnpm | CA Certs Env (`NODE_EXTRA_CA_CERTS`) |
| pip | CA Certs Env (`PIP_CERT`) |
| uv | CA Certs Env (`UV_NATIVE_TLS`) |
| poetry, conda, conan | CA Certs Env (`REQUESTS_CA_BUNDLE`) |
| nix | CA Certs Env (`NIX_SSL_CERT_FILE`) |
| hex/mix | CA Certs Env (`HEX_CACERTS_PATH`) |
| maven, gradle | JKS TrustStore (`MAVEN_OPTS`/`GRADLE_OPTS`) |
| bazel | JKS TrustStore (`bazel.bazelrc`) |

## Adding Custom Extensions

Extensions are Go types in `cmd/warden/`. To add one:

1. Create `cmd/warden/ext_myext.go` implementing the `Extension` interface
2. Add it to the `exts` slice in `cmd/warden/orchestrator.go`

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
