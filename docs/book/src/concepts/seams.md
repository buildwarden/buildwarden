# Component seams

A *seam* is a boundary between two of BuildWarden's components where another
implementation may take the place of one side: a relay written in another
language, a collector that stores outputs somewhere other than a disk, an
executor for another guest. Each seam is specified in its own document, with
numbered requirements, and comes with a conformance suite that checks an
implementation of either side against them.

The specifications of every seam but the ledger live in their own repository,
[buildwarden/buildwarden-specs](https://github.com/buildwarden/buildwarden-specs),
and are released one at a time, each under its own tags
`<directory>/vX.Y.Z`. Where this book's other pages describe a seam (the
[architecture](./architecture.md) among them), they explain it; the
specification is the contract.

## The seams

| Seam | Between | Specification | Implemented here by | Suite | Pinned at |
|---|---|---|---|---|---|
| 1, the claim | The claimant and the relay | [relay-claim](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/relay-claim/v1.md) ([OpenAPI](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/relay-claim/v1.openapi.json)) | `relaycfg/`, `cmd/relay/`; the claimant, `driver.Session` | [relay-claim cases](https://github.com/buildwarden/buildwarden-specs/tree/f3087fc53892/relay-claim/cases) | `relay-claim@v0.0.0-20261116100509-f3087fc53892` |
| 2, the outputs a collector stores | The relay and the collector | [collector-outputs](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/collector-outputs/v1.md) ([OpenAPI](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/collector-outputs/v1.openapi.json)) | `collector/` (`cmd/collector`); the relay's side, `relay/collector_client.go` | [collector-outputs cases](https://github.com/buildwarden/buildwarden-specs/tree/f3087fc53892/collector-outputs/cases) | `collector-outputs@v0.0.0-20261116100509-f3087fc53892` |
| 2, the inputs and signals a collector serves | The relay and the collector | [collector-inputs](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/collector-inputs/v1.md) ([OpenAPI](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/collector-inputs/v1.openapi.json)) | `collector/` (`cmd/collector`); the relay's side, `relay/collector_client.go` | [collector-inputs cases](https://github.com/buildwarden/buildwarden-specs/tree/f3087fc53892/collector-inputs/cases) | `collector-inputs@v0.0.0-20261116100509-f3087fc53892` |
| 3, the relay's guest API | The relay and the build environment | [relay-guest-api](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/relay-guest-api/v1.md) (OpenAPI: [artifacts](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/relay-guest-api/v1.artifacts.openapi.json), [context](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/relay-guest-api/v1.cwd.openapi.json), [control](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/relay-guest-api/v1.control.openapi.json)) | `relay/`, `cmd/relay/` | [relay-guest-api cases](https://github.com/buildwarden/buildwarden-specs/tree/f3087fc53892/relay-guest-api/cases) | `relay-guest-api@v0.0.0-20261116100509-f3087fc53892` |
| 4, the ledger | The relay and every reader | [Ledger Wire Format v1](../specs/build-ledger/v1.md) and its metadata schemas, in this repository until milestone M6 | `ledger/` | `specs/specs_test.go` and the ledger's own tests | Not pinned: it is in this repository |
| 5, the executor's `initialize` | The launcher and the executor | [executor-initialize](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/executor-initialize/v1.md) | `cmd/warden-io/` | None yet: its cases come with phase S2 | `executor-initialize@v0.0.0-20261116100509-f3087fc53892` |
| 6, the bootstrap config | Its producer and the executor, carried by the relay | [bootstrap-config](https://github.com/buildwarden/buildwarden-specs/blob/f3087fc53892/bootstrap-config/v1.md) | `bootstrap/` | [bootstrap-config cases](https://github.com/buildwarden/buildwarden-specs/tree/f3087fc53892/bootstrap-config/cases) | `bootstrap-config@v0.0.0-20261116100509-f3087fc53892` |

## Which version this repository implements

This repository pins one version of each of the six specifications, in a Go
module of its own, `tools/conformance/<directory>/go.mod`, which requires that
specification's module and names its command, `bw-conformance-<directory>`, in
a `tool` directive. The last column above is that pin, as
`<directory>@<version>`, and every link in a row names the ref the pin is at:
the tag `<directory>/vX.Y.Z` for a release, or, for a branch in progress that
pins an unreleased commit of the specifications, that commit. `main` pins
releases only.

The `conformance` workflow builds each pinned command and runs its suite
against the relay and the disk collector built from this repository, each
described by a target file in `tools/conformance/targets/`: the relay in host,
vm and container mode and as its container image, and the disk collector on
Linux, macOS and Windows. `make conformance` runs the same locally. A pin is
moved with `make pin-specs`, which rewrites the module and the refs on this
page together, and never with `go get`:

```sh
make pin-specs SPECS="relay-claim@v0.2.0"   # the named specifications, to releases
make pin-specs SPECS=latest                 # all six, to their newest releases
make pin-specs REF=<commit>                 # all six, to one unreleased commit
```
