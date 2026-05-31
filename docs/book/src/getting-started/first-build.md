# Your First Build

## Run a warden build

The basic flow is the same regardless of driver:

```sh
warden build .                           # Container driver (default)
warden build --driver qemu .             # QEMU driver
warden build --driver vz --script ./build.sh .  # VZ driver
```

For the container driver you can also point at a specific Dockerfile:

```sh
warden build ./my-project/Dockerfile.prod
```

## What you'll see

The output is consistent across all drivers:

```
[warden] Build ID: abc12345
[warden] ... (driver-specific startup messages) ...
warden-io: configuring network
warden-io: waiting for relay
warden-io: installing CA
warden-io: fetching build script
warden-io: running build.sh
... (build output) ...
[warden] Build complete
[warden] Output: warden-output
```

## Output directory

After the build completes (same structure for all drivers):

```
warden-output/
├── ledger                  # Cryptographic ledger (or ledger.zst if --compress)
├── ca.cert.pem             # Ephemeral CA cert
├── artifacts/              # Build outputs posted via warden-io post
│   └── myapp.whl
├── payloads/               # Content-addressed artifact storage
├── relay.log               # Relay event log (container driver only)
├── Dockerfile.submitted    # Original Dockerfile (container driver only)
├── Dockerfile.actual       # Rewritten Dockerfile (container driver only)
└── build.sh                # Translated build script (container driver only)
```

## Posting artifacts

To record a build output in the ledger, use `warden-io post` from within your build script or Dockerfile RUN step:

```sh
warden-io post /path/to/output.whl myapp-1.0.0.whl
```

The artifact is hashed, signed into the ledger, and saved in the output directory. This works identically in all drivers.

## Fetching context files

To pull files from the host into your build environment:

```sh
# Fetch a single file:
warden-io fetch config.json -o /tmp/config.json

# Fetch entire context:
warden-io fetch . -o /workspace
```

This works in all drivers — the relay serves context files over HTTP regardless of whether the build environment is a container or VM.

## Interactive debugging (container driver)

```sh
warden shell .
# Drops into isolated container — all traffic still audited
```

## Custom output location

```sh
warden build -o ./my-custom-output .
```

## Disabling compression

```sh
warden build --no-compress .
```
