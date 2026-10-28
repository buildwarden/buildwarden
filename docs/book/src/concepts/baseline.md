# Trust and the Build Baseline

Every HTTPS connection from the build goes through the relay, which intercepts
it with a certificate authority (CA) generated for that build. Tools in the
build have to trust that CA. `warden-io initialize` sets this up itself, inside
the build environment, before your build script runs. The only input it needs
from the host is the CA, which it fetches from the relay. See
[How It Works](./overview.md) for where this sits in a build.

## System trust

`initialize` installs the CA as a system root, in one of two modes. The
orchestrator chooses the mode and sends it in the build's bootstrap config
(`GET http://artifacts/bootstrap`), which the relay records in the ledger:

| Mode | Guests | Effect |
|------|--------|--------|
| `exclusive` | Linux (every driver) | The warden CA becomes the only trusted root: `/etc/ssl/certs` holds just it, and other known system bundles (`/etc/pki/...`, `/etc/ssl/cert.pem`) are replaced. A `ca-certificates` update hook restores this if the build reinstalls `ca-certificates`. |
| `append` | Windows, macOS | The warden CA is added to the existing roots (the Windows `Root` store, the macOS System keychain and bundle). |

Exclusive trust loses nothing. Every TLS connection the build makes is
intercepted by the relay, so no other root could ever validate a real
connection. It also makes it obvious when a tool ignores the system store: that
tool fails loudly instead of quietly trusting something else.

## The baseline

`initialize` applies the baseline to every build, on every driver. It is not
configurable. Each part is a small file or an environment variable, harmless
when the tool it serves is absent.

| What | Where | For |
|------|-------|-----|
| Java truststore (JKS) with the warden CA | `/etc/warden/certs.jks` (`%ProgramData%\warden\certs.jks` on Windows) | JVM tools |
| `MAVEN_OPTS`, `GRADLE_OPTS` | `-Djavax.net.ssl.trustStore=<that file>` | Maven, Gradle |
| Bazel system rc | `/etc/bazel.bazelrc` | Bazel's host JVM |
| `SSL_CERT_FILE`, `CURL_CA_BUNDLE` | the system CA bundle | OpenSSL consumers, Ruby, Erlang, PHP composer, libcurl |
| `REQUESTS_CA_BUNDLE`, `PIP_CERT` | the system CA bundle | pip, poetry, conda, conan |
| `NODE_EXTRA_CA_CERTS` | the system CA bundle | npm, yarn, pnpm, bun |
| `CARGO_HTTP_CAINFO` | the system CA bundle | cargo |
| `NIX_SSL_CERT_FILE`, `HEX_CACERTS_PATH` | the system CA bundle | nix, hex |
| `UV_NATIVE_TLS=1` | | uv |
| `SOURCE_DATE_EPOCH=0` | | reproducible timestamps |

The baseline variables are set on the build script's process, so an `ENV` line
in your Containerfile (or an `export` in your script) overrides them. If you set
`MAVEN_OPTS` yourself, include the truststore flags or Maven will not trust the
relay.

The JKS file is written by `warden-io` directly; no `keytool` is needed in the
image. Its bytes match what `keytool -importcert -storetype JKS` writes, and the
same CA always produces the same file.

## Package manager compatibility

| Manager | Trusts the relay through |
|---------|--------------------------|
| apt, apk, dnf, go modules, nuget/.NET | system trust |
| gem/bundler, composer | `SSL_CERT_FILE`, `CURL_CA_BUNDLE` |
| npm, yarn, pnpm | `NODE_EXTRA_CA_CERTS` |
| pip, poetry, conda, conan | `PIP_CERT`, `REQUESTS_CA_BUNDLE` |
| uv | `UV_NATIVE_TLS` |
| cargo | system trust, `CARGO_HTTP_CAINFO` |
| nix, hex/mix | `NIX_SSL_CERT_FILE`, `HEX_CACERTS_PATH` |
| maven, gradle | `MAVEN_OPTS`/`GRADLE_OPTS` and the JKS |
| bazel | `/etc/bazel.bazelrc` and the JKS |

## Adding support for another tool

Support for a tool is added to the baseline in `warden-io`
(`cmd/warden-io/baseline.go`), so it ships with the executor and reaches every
driver. Anything specific to one build belongs in that build's Containerfile.
There is no plugin mechanism for setup scripts: the build environment runs no
setup code that `warden-io` did not ship with (BOOT-05 in
`docs/development/bootstrap.md`).
