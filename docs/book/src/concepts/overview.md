# How It Works

This page describes each core component as pseudocode: what it does, in what
order, and what it talks to. It is kept in step with the code (FLOW-15 in
`docs/development/process.md`), so you can read how a build runs without reading
the source. [Architecture](./architecture.md) has the topology diagrams and the
endpoint tables.

## The shape of a build

```
  HOST                                              BUILD ENVIRONMENT
  warden (orchestrator)                             (container or VM)
    |-- collector  <==== tunnel ====  relay  <====  warden-io initialize
    |   serves inputs,                 the only       fetches every input,
    |   lands outputs                  network path   runs the build script
    '-- driver: isolate, deliver warden-io, start (then only watches)
```

Three rules hold for every driver (the full model is
[`docs/development/bootstrap.md`](https://github.com/buildwarden/buildwarden/blob/main/docs/development/bootstrap.md)):

1. The build environment is isolated before any of its code runs.
2. The host delivers exactly one thing into it: the `warden-io` executable.
3. After starting it, the host does nothing inside it. Everything else (the CA,
   the build script, your files) comes through the relay, which records it.

## warden build (orchestrator, `cmd/warden`)

```
warden build <context>:
    req := resolve flags and config (driver, runtime, output dir, ...)
    driver := select(req.driver)            # container | qemu | vz | hyperv
    driver.StartBuild(req)                  # below; every driver has this shape
    print the result; exit with the build script's exit code
```

Every driver's `StartBuild` follows the same skeleton. Only the marked steps
differ per driver.

```
StartBuild(req):
    script := translate(req.Containerfile, req.ContextDir)   # see Translator
    session := StartSession(output dir, context, script, guest OS)
    relay := start the relay, unconfigured                 # per driver
    session.ClaimRelay(relay)                              # tunnel + config
    isolate the build environment                          # per driver
    deliver warden-io into it                              # per driver
    start it                                               # per driver
    session.Wait()          # until warden-io reports exit, or a failure
    teardown; finalize output (optional zstd)
    if script was translated from req.Containerfile:      # not vz
        keep both under <output>/inputs/ (Dockerfile, build.sh; ORCH-02)
```

## Session and collector (`driver/session.go`, `collector/`)

The host side of one build, the same for every driver.

```
StartSession(cfg):
    collector := new collector(
        output dir,
        build script      -> GET /v1/build-script
        bootstrap config  -> GET /v1/bootstrap   ({"version":1,"trust":{"mode":...}})
        build context     -> GET /v1/context/<path>
    )

ClaimRelay(addr, token):
    tunnel := POST addr/v1/tunnel           # host dials the relay, never the reverse
    serve collector over tunnel (HTTP/2)
    identity := POST addr/v1/claim {collector URL = the tunnel, token, capture mode,
                                    upstream CAs, environment record}
    wait for the relay's POST /v1/ready
    write identity.CACert to <output>/ca.cert.pem

collector, for the life of the build:
    PUT  /v1/ledger, /v1/artifacts/*, /v1/captures/*, /v1/environment  -> land on disk
    POST /v1/output       -> append to build-output.log (and tee to your terminal)
    POST /v1/heartbeat    -> note the guest is alive
    POST /v1/complete     -> record the exit code; Wait returns

Wait():
    return when /v1/complete arrives (exit code 0 => success)
    fail if the guest stops heartbeating, or the tunnel drops
```

## Translator (`driver/script`)

Your Containerfile becomes a shell script on the host, because translating
needs the build context.

```
translate(containerfile, context):
    for each instruction:
        FROM x       -> the base image (one stage only)
        RUN cmd      -> cmd
        ENV k=v      -> export k="v"            ($VAR expands, as in Docker)
        ARG k=v      -> : ${k:=v}; export k
        WORKDIR d    -> mkdir -p d; cd d
        COPY src dst -> expand src against the context ON THE HOST
                        (files, directories, globs; .git skipped), then for each
                        file: warden-io fetch <context path> -o <destination>
        USER, SHELL, ADD, a second FROM -> error before anything boots
        EXPOSE, CMD, ENTRYPOINT, LABEL, ... -> ignored
```

Each `warden-io fetch` in the script pulls one file through the relay, which
records its bytes.

## Container driver (`driver/container`)

```
start the relay container on a per-build network; claim it

isolate:
    holder := run warden-holder              # does nothing; owns the network
              --network <build net> --cap-drop ALL --read-only
    run --privileged warden-netns            # joins the holder's namespace,
        --network container:<holder>         # applies iptables, exits:
                                             # 80/443/53 -> relay, all else DROP

deliver:
    build := create <your FROM image>        # created, NOT started
             --network container:<holder>    # inside the isolated namespace
             --user 0
             --entrypoint /.warden/warden-io  initialize --gateway=<relay> --workdir=/work
    cp - build:/  <  tar{ /.warden/warden-io, mode 0755 }

start:
    start build                              # warden-io initialize runs as PID 1

watch:
    wait for the session's completion, or the container exiting without one
```

The image's own `ENTRYPOINT` never runs, and nothing from the image exists
until the network is already isolated. The holder and sidecar images are
published with each release and run by digest.

`warden shell` does the same with `initialize --hold`, then opens a shell in the
held environment.

## VM drivers (`driver/qemu`, `driver/hyperv`, `driver/vz`)

```
start the relay VM (or, on VZ, a host relay process); claim it

isolate:   the build VM's only network interface leads to the relay
           (QEMU socket pair, Hyper-V private switch, VZ socketpair)

deliver:   Linux guest   -> cloud-init CIDATA disk carrying warden-io
           Windows guest -> WARDEN seed disk carrying warden-io.exe and warden-run.ps1
           macOS (VZ)    -> warden-io baked into the prepared image (a known deviation)

start:     boot the VM. Its launcher runs warden-io initialize:
           Linux   -> cloud-init runcmd
           Windows -> the warden-run startup task in the prepared image
           macOS   -> a launchd daemon

watch:     wait for the session's completion
```

## warden-io initialize (`cmd/warden-io`)

The only program the host puts in the build environment. It sets the
environment up from the relay, then runs your build.

```
initialize --gateway=<relay> [--ip=<static>] [--workdir=<dir>] [--hold]:
    point DNS (and on VMs, the address and route) at the relay
    wait for GET http://<relay>:8300/health

    ca  := GET http://artifacts/ca.pem
    cfg := GET http://artifacts/bootstrap     # 404 => empty config
    reject cfg if it has an unknown version, field or value

    install the CA as a system root:
        exclusive (Linux default) -> the warden CA becomes the ONLY root, and a
                                     ca-certificates update hook keeps it so
        append                    -> add it to the existing roots

    baseline, always (derived from the CA alone):
        /etc/warden/certs.jks            Java truststore (JKS, matches keytool)
        /etc/bazel.bazelrc               points Bazel's JVM at it
        MAVEN_OPTS, GRADLE_OPTS          point Maven and Gradle at it
        SSL_CERT_FILE, REQUESTS_CA_BUNDLE, NODE_EXTRA_CA_CERTS, PIP_CERT,
        CURL_CA_BUNDLE, CARGO_HTTP_CAINFO, NIX_SSL_CERT_FILE, HEX_CACERTS_PATH
                                         the system CA bundle
        UV_NATIVE_TLS=1, SOURCE_DATE_EPOCH=0

    mkdir and cd into the workdir
    if --hold: write the environment to /etc/warden/shell.env and wait forever

    script := GET http://artifacts/build-script
    run script, with warden-io on PATH:
        stdout/stderr -> your console, and POST :8300/v1/output in chunks
        every 2s      -> GET http://artifacts/heartbeat
    POST http://artifacts/exit?code=<script's exit code>
    exit with that code
```

Your script's own `ENV` lines override any baseline variable. The other
subcommands are what the script calls: `warden-io fetch <path> -o <dest>` (a
context file from `http://cwd/<path>`) and `warden-io post <file>` (an artifact
to `http://artifacts/<name>`).

## Relay (`relay/`, `cmd/relay`)

```
relay --mode=container|vm|host:
    bind DNS (:53), HTTP (:80), HTTPS (:443), control plane (:8300)
    accept ONE claim on :8301:
        open the tunnel; take the config; generate the per-build MITM CA and the
        ledger key; return both; sign the environment record as the first ledger
        record; POST /v1/ready to the collector
    stop listening for claims

    for each guest request:
        DNS: artifacts, cwd -> the relay itself; anything else -> upstream, recorded
        http://artifacts/
            /ca.pem, /bootstrap, /build-script -> serve (pulled from the collector;
                                                  bootstrap and script recorded)
            /heartbeat -> forward to the collector
            /exit      -> flush the ledger, POST /v1/complete
            POST /<name> -> stream the artifact to the collector, recorded
        http://cwd/<path> -> the context file from the collector, recorded
        anything else -> proxy upstream (HTTPS through the MITM CA), recorded;
                         captures streamed to the collector

    the ledger is streamed to the collector as it is written, so even a relay
    that dies mid-build leaves a verifiable (truncated) ledger
```

## Namespace holder (`cmd/warden-pause`)

```
warden-pause:
    wait for SIGTERM or SIGINT
    exit 0
```

It runs before the container driver's network is isolated, so it has no
network code, no shell, and no capabilities.
