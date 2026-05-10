# Examples

BuildWarden ships with example Dockerfiles demonstrating progressively complex builds.

## Running Examples

```sh
warden build examples/Dockerfile.simple
warden inspect warden-output
```

## Available Examples

### Dockerfile.simple

Builds a Python `requests` wheel from source using pip.

- **Demonstrates:** Basic end-to-end flow — package fetch, wheel build, artifact posting
- **Time:** ~1 minute
- **Network fetches:** Docker Hub, Debian repos, PyPI
- **Artifact:** `requests-2.32.3-py3-none-any.whl` (deterministic with SOURCE_DATE_EPOCH)

### Dockerfile.expanded

Same wheel build plus test suite and type checking.

- **Demonstrates:** Ledger truncatability — test/lint network fetches appear as separate entries
- **Time:** ~3 minutes
- **Extra fetches:** pytest, mypy, type stubs from PyPI

### Dockerfile.cryptography

Builds Python `cryptography` from source (includes Rust/Cargo backend).

- **Demonstrates:** Multi-ecosystem auditing — apt packages, Rust crates, pip packages all in one ledger
- **Time:** ~3 minutes
- **Network fetches:** Debian repos, crates.io (Cargo), PyPI

### Dockerfile.pytorch-aarch64

Builds PyTorch from source with CUDA support on ARM64.

- **Demonstrates:** Large-scale build with hundreds of network fetches
- **Time:** ~3 hours
- **Network fetches:** NVIDIA CUDA, GitHub (submodules), PyPI
- **Requires:** ARM64 host, ~50GB disk space

## Writing Your Own

Any standard Dockerfile works. To post build outputs to the ledger:

```dockerfile
RUN curl -fsSL -X POST --data-binary @/path/to/output \
    "http://artifacts/my-artifact-name"
```

The `artifacts` hostname is reserved — it resolves to the relay and triggers artifact recording in the ledger.

## Reproducibility Demo

The `simple` and `expanded` examples both pin `requests==2.32.3` with `SOURCE_DATE_EPOCH` set. Despite fetching source from different places (PyPI sdist vs git clone), they produce byte-identical wheels — demonstrating that the ledger can prove two independent builds created the same artifact.
