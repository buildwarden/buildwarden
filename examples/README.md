# Examples

Each Dockerfile demonstrates BuildWarden with a progressively more complex build. Run any of them with:

```sh
warden build examples/Dockerfile.simple
```

## Dockerfiles

| File | What it builds | What it demonstrates | Time |
|------|---------------|---------------------|------|
| `Dockerfile.simple` | Python `requests` wheel from sdist | Minimal end-to-end: pip fetch, wheel build, artifact posting | ~1 min |
| `Dockerfile.expanded` | Same wheel + tests + type-check | Ledger truncatability — test/lint fetches are separate entries from the build | ~3 min |
| `Dockerfile.cryptography` | Python `cryptography` from source | Multi-ecosystem auditing: Rust/Cargo crates + pip + apt + OpenSSL | ~10 min |
| `Dockerfile.pytorch-aarch64` | PyTorch from source (ARM64, CUDA) | Large-scale build with hundreds of network fetches, conda + pip + cmake | ~3 hrs |
| `Dockerfile.tfexample-aarch64` | TensorFlow from source (ARM64, CUDA) | Bazel remote-fetch auditing, artifact posting of built wheel | ~4 hrs |

## Inspecting results

After a build completes, inspect the ledger:

```sh
warden inspect /tmp/warden-ledger-*/ledger
```

Or for machine-readable output:

```sh
warden inspect --json /tmp/warden-ledger-*/ledger
```

## Adding your own

Any standard Dockerfile works. To post build artifacts back to the ledger, use the reserved `artifacts` hostname:

```dockerfile
RUN curl -X POST --data-binary @output.whl "http://artifacts/output.whl"
```

The `.warden/` directory is automatically injected into the build context with CA certificates and extension scripts. You don't need to reference it explicitly.
