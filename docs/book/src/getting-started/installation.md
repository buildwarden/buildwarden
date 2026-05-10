# Installation

## Prerequisites

BuildWarden requires a container runtime. Any of the following will work:

- [Finch](https://github.com/runfinch/finch) (preferred on macOS)
- [Docker](https://docs.docker.com/get-docker/)
- [Podman](https://podman.io/)

## One-liner Install

```sh
curl -sSfL https://raw.githubusercontent.com/buildwarden/buildwarden/main/install.sh | sh
```

This detects your OS and architecture, downloads the latest release, and installs to `/usr/local/bin/`.

## From Releases

Download the appropriate binary from [GitHub Releases](https://github.com/buildwarden/buildwarden/releases/latest):

```sh
# macOS (Apple Silicon)
tar xzf buildwarden_*_darwin_arm64.tar.gz
mv warden /usr/local/bin/

# Linux (x86_64)
tar xzf buildwarden_*_linux_amd64.tar.gz
mv warden /usr/local/bin/
```

## From Source

```sh
git clone https://github.com/buildwarden/buildwarden.git
cd buildwarden
make build
# Binary is at ./warden
```

Requires Go 1.26+.

## Verify Installation

```sh
warden --version
warden --help
```

BuildWarden auto-detects your container runtime (trying finch, docker, podman in order). Override with `--runtime` or the `WARDEN_CTR_CLI` environment variable.
