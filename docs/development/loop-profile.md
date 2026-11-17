# The Loop Profile

### PROFILE-01 — This repository's loop profile

**Severity:** MUST
**Check:** agent-judgment

The table below is `buildwarden/buildwarden`'s profile under the Loop
Contract (LOOP-01). Where an entry names a file, that file holds the
fact. An entry that no longer matches the repository is a finding, and
the change that makes it stale changes the entry.

The agent part: Review judges that the diff leaves no entry stale; the
design review of a change to this rule judges that every term is filled
or `none` (LOOP-01).

| Term | This repository's entry |
|---|---|
| Component | `buildwarden`, in `buildwarden/buildwarden`: the kit, the orchestrator, the collector, the executor, the drivers and, until every driver has adopted the Rust relay, the Go relay. |
| Trunk | `main`. |
| Toolchain | Go, at the version in each module's `go.mod`; `make`; `golangci-lint`. |
| Rules | `docs/development/`. The files and prefixes are the rows of `README.md`'s ID scheme table whose owner is `buildwarden/buildwarden`. |
| Designs | `docs/design/<item>.md`, written from `docs/design/TEMPLATE.md`, on the branch `feat/<item>`. The template's own sections are the ones it ties to a project rule: **Defaults and overrides** (GLOBAL-09 to GLOBAL-11), **Cross-driver impact** (ORCH-01), **Bootstrap comparison** (BOOT-07), **Security note** (GLOBAL-06). |
| Impact vocabulary | The keys and values in `docs/design/TEMPLATE.md`'s front-matter. The driver names, the shared packages that declare every dependent driver, and the seam names are the `drivers` and `seams` lists of `tools/tier1/rules.yaml` (ORCH-01, GLOBAL-08). The `security_boundary` values are GLOBAL-06's. |
| Specifications | `specs/`: the ledger wire format and the metadata and environment schemas (RELAY-05), mirrored in the book. For the seams of GLOBAL-08, their contract documents are the `seams` list of `tools/tier1/rules.yaml` and their tests are TEST-05's. The claim protocol, the collector protocol, the relay guest API, the `initialize` contract and the bootstrap config are specified in `buildwarden/buildwarden-specs`, each specification released under its own version. This repository implements them and pins a release of each specification in `tools/conformance/<specification>/go.mod`; those files are their contracts in that list (GLOBAL-08). Each pin is a release tag on `main` and on a pull request to it; a branch may pin a commit of an unmerged specifications branch while its work is in progress. |
| Fast gate | `make lint` and `make test` (TEST-01). `make lint` is also run as `GOOS=<os> make lint` for each platform whose build-tagged files the change touches (FLOW-17). |
| Checks | The table below. |
| Test tiers | TEST-02, which says where each tier runs. |
| Native hosts | The host names in the loop's `hosts.yaml`. All of them serve this repository's drivers today. |
| Reference documents | The surface and its documents are the `surface` entry of `tools/tier1/rules.yaml` (UX-01, ORCH-03). Guides: `docs/book/src/guide/`, `docs/book/src/getting-started/`, `examples/`, `README.md`. |
| Sweep | `make deadcode`, on Linux, macOS and Windows. Kept with a stated reason: the exported API of `relay/`, `collector/`, `ledger/`, `relaycfg/` and `bootstrap/`, and the cgo-only code of `driver/vz`. The overview is `docs/book/src/concepts/overview.md`. |
| Boundary rule | GLOBAL-06. |
| Flaky-test rule | TEST-04. |
| Rule and machinery paths | Rule changes: `docs/development/`, `tools/tier1/`, `tools/workflow/check/`, the rule-running jobs of `.github/workflows/`, `docs/design/TEMPLATE.md`. Machinery: `.claude/` and `tools/workflow/` outside `check/`. |
| Areas | The loop's `areas.yaml`, in the queue state on the `workflow` branch: this repository's areas, their paths and modes, which area reaches which, the area each `drivers` and `surfaces` value stands for, the areas every driver shares, and the seams that have a specification. It is not one of this repository's rule files: it changes in an attended session, on the human's word (FLOW-05). `tools/tier1/area-edges.sh` prints this repository's dependency edges, which the loop compares the map's `reaches` with. |
| Release | A `v*` tag push runs the `release` workflow (GoReleaser), whose `tier1` job is the gate's deterministic half (LINT-03), and the `relay-image` workflow. The human's sign-off of a tier-2 report over the release diff has no wired step yet. |
| Consumables | Releases: the `v*` tags. A release that carries the files `tools/conformance/<specification>/go.mod` implements each seam specification at the version its file pins. Consumes: the releases of the seam specifications in `buildwarden/buildwarden-specs`, each with its conformance suite, at the versions in those files. |
| Sessions | `.entire/settings.json`, whose `checkpoint_remote` is `buildwarden/buildwarden-sessions`; `tools/workflow/flow18-check.sh`; the `flow18` workflow. |

The Checks entry, by class. Tier 1 is deterministic and blocks; tier 2 is
the `wf-review` agent, report-only (LINT-07):

| Class | Where its checks are defined | Tier |
|---|---|---|
| Build | The `build`, `tidy` and `book` jobs of `.github/workflows/main.yml` | 1 |
| Specification | The coupling checks that `tools/tier1/rules.yaml` maps to RELAY-05, GLOBAL-08, EXEC-04, UX-01, UX-03, ORCH-03 and BOOT-07; the `conformance` workflow, which runs TEST-05's suites at the pinned specification releases and whose `pin` job requires release tags on a pull request; the contract tests of TEST-05 that run in `make test`; the `keytool` job | 1 |
| | The judgment half of those rules | 2 |
| Safety | `make lint` (`golangci-lint`, and tier 1's analyzers as `rules.yaml` maps them) and `make test`; the `integration` workflow (TEST-02's tier 1) | 1 |
| | Native validation on a host | 1; blocking or tracked per design (FLOW-09) |
| | Every `agent-judgment` rule and the judgment half of every hybrid rule | 2 |
| Process | Tier 1's `rule-index`, `design-front-matter`, `drivers-claim` and `suppression` checks; `wf`'s done-when checks and `wf validate`; `make enforcement`; `tools/workflow/flow18-check.sh` | 1 |

Declared as not blocking: `tests-with-change` (GLOBAL-04), a hint; `make
deadcode` (FLOW-15), a report that Publish judges; and the `firecracker`
workflow, until its header's condition is met.
