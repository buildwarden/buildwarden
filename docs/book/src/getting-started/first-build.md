# Your First Build

## Run a warden build

If you have a `Dockerfile` in your current directory:

```sh
warden build
```

Or point at a specific Dockerfile or directory:

```sh
warden build ./my-project
warden build ./my-project/Dockerfile.prod
```

BuildWarden will:

1. Build the relay (a lightweight MITM proxy)
2. Allocate an isolated network subnet
3. Start the relay and build containers
4. Run your Docker build with all traffic flowing through the relay
5. Collect the ledger and artifacts into `warden-output/`

## What you'll see

```
[warden] Build ID: abc12345
[warden] Building relay from source...
[warden] Allocating network...
[warden] Starting relay...
[warden] Starting build container...
[warden] Configuring network isolation...
[warden] Environment ready
[build]  Starting build...
... (your normal Docker build output) ...
[warden] Build complete
[warden] Tearing down environment...
[warden] Output: warden-output
```

## Output directory

After the build completes:

```
warden-output/
├── ledger.zst              # The cryptographic ledger (compressed)
├── Dockerfile.submitted    # Your original Dockerfile
├── Dockerfile.actual       # The rewritten version BuildWarden ran
├── ca.cert.pem.zst         # Ephemeral CA cert used for TLS interception
├── relay.log               # Relay event log (DNS, HTTP, artifacts)
└── artifacts/              # Build outputs you posted to the ledger
    └── myapp.whl
```

## Posting artifacts

To record a build output in the ledger, POST it to the reserved `artifacts` hostname from within your Dockerfile:

```dockerfile
RUN curl -fsSL -X POST --data-binary @/output/myapp.whl \
    "http://artifacts/myapp-1.0.0.whl"
```

The artifact is hashed, signed into the ledger, and saved in the output directory.

## Custom output location

```sh
warden build -o ./my-custom-output .
```

## Disabling compression

```sh
warden build --no-compress .
```
