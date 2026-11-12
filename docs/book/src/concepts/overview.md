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
    VM drivers: req.BuildLink := --build-link, else [vm] build_link (checked as
                the file loads), else 100.127.255.252/30; each checked /30,
                disjoint from BuildWarden's own ranges, before any driver work
                req.DNS := [vm] dns (checked as the file loads: IPv4, not
                unspecified, multicast or broadcast, not empty), else none;
                the container driver gets neither
    driver := select(req.driver)            # container | qemu | vz | hyperv | firecracker
    driver.StartBuild(req)                  # below; every driver has this shape
    print the result; exit with the build script's exit code
```

Every driver's `StartBuild` follows the same skeleton. Only the marked steps
differ per driver.

```
StartBuild(req):
    script := translate(req.Containerfile, req.ContextDir)   # see Translator
    if script was translated: keep both under <output>/inputs/  # ORCH-02; not vz
    session := StartSession(output dir, context, script, guest OS)
    VM drivers: refuse a build link a host interface holds, and a launcher
                asset whose link-channel stamp is missing or old
    relay := start the relay, unconfigured                 # per driver
    session.ClaimRelay(relay, build link, req.DNS)         # tunnel + config;
                                                           # container: neither
    isolate the build environment                          # per driver
    deliver warden-io into it                              # per driver
    start it                                               # per driver
    session.Wait()          # until warden-io reports exit, or a failure
    teardown; finalize output (optional zstd)
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

ClaimRelay(addr, token, link, oldRelayFix):
    tunnel := POST addr/v1/tunnel           # host dials the relay, never the reverse
    serve collector over tunnel (HTTP/2)
    identity := POST addr/v1/claim {collector URL = the tunnel, token, capture mode,
                                    upstream CAs, environment record,
                                    build_link {prefix, relay, guest} when given,
                                    upstream_dns = req.DNS when given}
        a relay that refuses any field as unknown (it predates build_link or
        upstream_dns): a fix naming oldRelayFix
    # the relay answers only once its build segment is configured
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

## VM drivers (`driver/qemu`, `driver/hyperv`, `driver/vz`, `driver/firecracker`)

```
before anything boots:
    vz base image: --image, else the image cache's active image (the one
    `image prepare` made active last; with no pointer, the newest prepared)
    refuse a build link that holds a host interface address
    check the launcher's link-channel stamp: the qemu or firecracker direct-boot
    initramfs, the hyperv relay boot VHDX, the vz prepared image; missing or old
    -> a fix naming its one-time rebuild (vz: the active image -> `image
    prepare` with no source; another cache image -> `image prepare <dir>`, or
    omit --image; an --image disk outside the image cache, which cannot be
    re-prepared in place -> omit --image)

the relay's resolvers (driver/resolvers), before any VM:
    [vm] dns set -> Check: every entry passes the driver's rule, in order,
                    used exclusively; the first that fails refuses the build
    unset        -> Select(the host's list, the driver's rule): the ones that
                    pass, in order; none -> a fix naming each excluded server,
                    an off-host DNS server and [vm] dns
    req.DNS := that list; the claim carries it as upstream_dns
        driver       host source                     excluded
        hyperv       the IPv4 DNS servers of the     loopback, link-local, host
                     lowest-metric default route's   addresses, the NAT prefix,
                     interface                       the build link
        firecracker  /etc/resolv.conf, or resolved's loopback, link-local, host
                     upstreams behind its stub       addresses, the build link
        qemu         /etc/resolv.conf (macOS or      loopback, link-local,
                     Linux)                          10.0.2.0/24, the build link
        vz           the Mac's /etc/resolv.conf      the build link

start the relay VM (or, on VZ, a host relay process); claim it with the build
link and the resolvers
    firecracker: first take a free network slot made by `warden setup` (a host
                 lock), kill VMMs a dead warden left on it, and claim the relay
                 at the slot's upstream address
    hyperv setup: refuse a --nat-prefix that overlaps the build link; make or
                 adopt <network>-nat on --nat-prefix, refusing another NAT that
                 owns the prefix or <network>-nat on another (making it leaves
                 --host-ip the only IPv4 address on vEthernet (<network>-nat));
                 make the
                 durable strategy's slot switches <network>-0..N-1 (--slots,
                 default 4), trim unattached extras, retire an unattached old
                 <network> switch
    hyperv build (StartBuild):
        hold the host lock "hyperv-build-<id>" from the build ID's creation
            to StartBuild's return
        the network strategy: elevated -> a per-build network; else the
            privileged service at this protocol -> a per-build network through
            it; a service at another protocol (or failing its ping) -> a fix
            naming its reinstall, never the durable network; else durable
            (doctor reads the service manager when the pipe does not open:
            installed but stopped -> Start-Service WardenHyperV, never
            "not installed"; a read refused or failed -> "state could not
            be read", naming Get-Service WardenHyperV; only "does not exist"
            is not installed)
        the relay boot VHDX's stamp must be 2, else a fix naming refresh-cache.sh
        the NAT read back from the host: name, prefix, host IP (two host
            addresses in the prefix -> refused as ambiguous)
            durable:   <network>-nat and its slot switches; none -> warden setup
            ephemeral: a per-build Private switch; the dev switch's NAT if
                       present, else the one owning the default prefix, else new
        a NAT prefix that overlaps the build link -> a fix naming that NAT
            (config load checks no Hyper-V range; this is the one check),
            run first on the NAT read back before the seed ISO and any
            standup (none yet -> the default prefix, named), then on the NAT
            the build took
        the resolvers, as above; token := a fresh random claim token
        under a host lock ("hyperv-network"):
            relay address := the lowest free host in the NAT prefix (not the
                             network, broadcast or host IP, nor any warden-relay-*
                             VM's Notes); none free -> a fix naming the holders
            durable: the lowest slot switch with no VM adapter attached;
                     none -> a fix naming the holders and --slots
            seed ISO (WARDENRELAY) := warden-relay.conf with warden.seed=1,
                     warden.net=static, warden.nat_ip, warden.nat_prefix,
                     warden.nat_gw, warden.claim_token  (pre-claim values only)
            create the relay VM, off: the seed DVD, the build NIC on the slot,
                     Notes "warden-relay-ip=<ip> warden-relay-switch=<switch>"
        start the relay VM; claim it at <relay address>:8301 with the token
        (Bearer on the claim and the tunnel)
        every seed ISO (the build's, the relay's, a prepare's) is made under
        a host lock ("hyperv-iso"), held only around IMAPI2
        the build VM joins the relay's switch; before it starts, the slot's
        adapters must be exactly the relay's build NIC and the build VM's,
        else refuse; removing the relay VM frees its address and slot
    hyperv clean (`warden clean --driver hyperv`): for each 8-character build
        ID among warden-build-*, warden-relay-* VMs and warden-hyperv-* work
        dirs whose "hyperv-build-<id>" lock it gets (not running): remove its
        VMs, then its ephemeral warden-<id> switch, then its work dirs; the
        NAT, the slot switches (<network>-<i>) and prepare VMs stay, so this
        frees a slot a --keep-vms build or a killed warden held
    hyperv host locks (driver.HostLock / TryHostLock, all "hyperv-<what>"):
        "hyperv-build-<id>"  one build's whole run; clean skips a held one
        "hyperv-network"     relay address and slot, through the relay VM's
                             creation
        "hyperv-iso"         IMAPI2 seed ISO creation
        "hyperv-prepare"     image prepare; a build tries it to see whether a
                             background refresh is already running
    hyperv PowerShell (runPS): a failed script's error is its stderr, else
        the exit status with the last 10 lines of stdout (psError); every
        script that changes the host (setup's slots, EnsureNetwork,
        TeardownNetwork, CreateVM, CreateRelayVM, RemoveVM, CreateDiffDisk,
        the seed ISO) prints DONE last and runs through runPSDone, which
        refuses output without it, naming that tail (psDoneCheck), since
        powershell -Command exits with the success of the last statement,
        which a suppressed lookup that finds nothing fails
    the build VM boots only after the claim returns

isolate:   the build VM's only network interface leads to the relay
           (QEMU socket pair, Hyper-V private switch, VZ socketpair,
            Firecracker: a TAP on the slot's host bridge, whose only other
            port is the relay and which drops every frame bound for the host)

deliver:   Linux guest   -> cloud-init CIDATA disk carrying warden-io
                            (Firecracker: a read-only drive beside a per-build
                             raw rootfs copy, or, with no image, warden-io
                             appended to a launcher-only initramfs)
           Windows guest -> WARDEN seed disk carrying warden-io.exe and warden-run.ps1
           macOS (VZ)    -> warden-io baked into the prepared image (a known deviation)

address:   the guest takes the build link per build, nothing baked:
           Linux seeded guests (cloud-init network-config)
                      -> rendered from the link
           Windows    -> the relay's DHCP lease alone: warden-run.ps1 runs
                      initialize --gateway=<relay> with no --ip; no lease ->
                      the relay wait fails naming the NIC's address and origin
           direct boot (qemu, firecracker) -> udhcpc takes the relay's lease;
                      it must agree with warden.link_ip / warden.link_gw on the
                      kernel command line (else stop); no lease -> the keys;
                      neither -> stop
           macOS (VZ) -> warden-boot takes the host relay's DHCP lease

start:     boot the VM. Its launcher runs warden-io initialize:
           Linux   -> cloud-init runcmd (Firecracker: ending in reboot -f), or
                      the direct-boot launcher's exec of initialize
           Windows -> the warden-run startup task in the prepared image
           macOS   -> a launchd daemon

watch:     wait for the session's completion
           firecracker: the build VM's process exiting allows a 10 s grace for
                        the completion; the relay VM's exiting fails at once
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
        open the tunnel; take the config; invalid build_link -> 400, still
        claimable
        configure the build segment (the mode's start step):
            vm:        build_link -> refuse (400, still claimable) if another
                                     interface holds an address or route in it;
                                     flush the build interface's IPv4, add
                                     relay/len (netlink); serve DHCP bound to
                                     that interface: one fixed lease (guest,
                                     mask, relay as router and DNS, infinite)
                       none       -> keep the interface's address; none there ->
                                     400 "build_link is required"
            host:      netstack, ARP, DHCP and listeners from build_link, else
                       --subnet
            container: build_link -> 400
        resolvers (every mode), fixed for the build:
            upstream_dns in the claim -> checked (non-empty IPv4, not
                unspecified, multicast or broadcast, not in build_link; else
                400, still claimable) and used exclusively
            absent -> the usable nameservers of its own /etc/resolv.conf (not
                its own address, 127.0.0.1 or the build link); none -> 400
                naming upstream_dns
            log "upstream resolvers: <list> (claim | resolv.conf)"
        generate the per-build MITM CA and the ledger key; return both; sign the environment record as the first ledger
        record; POST /v1/ready to the collector
    stop listening for claims

    for each guest request:
        DNS: artifacts, cwd -> the relay itself; anything else -> the resolvers,
             asked in order (the next when one does not answer), recorded
        http://artifacts/
            /ca.pem, /bootstrap, /build-script -> serve (pulled from the collector;
                                                  bootstrap and script recorded)
            /ca.crl    -> the CA's empty CRL, which every leaf names (not recorded)
            /heartbeat -> forward to the collector
            /exit      -> flush the ledger, POST /v1/complete
            POST /<name> -> stream the artifact to the collector, recorded
        http://cwd/<path> -> the context file from the collector, recorded
        anything else -> proxy upstream (HTTPS through the MITM CA), recorded;
                         captures streamed to the collector; a dial into the
                         build link is refused (vm and host), as are host mode's
                         SSRF blocks; its own lookups (upstream dials, a collector
                         host name) go through the same in-order forwarder

    the ledger is streamed to the collector as it is written, so even a relay
    that dies mid-build leaves a verifiable (truncated) ledger
```

## Relay VM init (`tools/relay-vm/init`)

The init of every vm-mode relay VM (qemu, Hyper-V, Firecracker). It carries
only the relay's pre-claim values; the build link and the resolvers come in
the claim.

```
read_keys(words):            # one reader for both sources; never evaluated
    warden.net=dhcp|static, warden.nat_ip=<quad>, warden.nat_prefix=1..30,
    warden.nat_gw=<quad>, warden.claim_token=<64 hex>, warden.seed=1
    a malformed value, or warden.dns (retired for upstream_dns) -> stop
qemu, Firecracker: read_keys(/proc/cmdline)
Hyper-V: the seed alone: any warden.* word on /proc/cmdline -> stop, naming it
         load the storage, CD-ROM and ISO 9660 modules; wait <= 10 s for the
         WARDENRELAY CD; copy warden-relay.conf; unmount; read_keys(it)
         no CD, no file or no warden.seed=1 -> stop
bring the build link's interface up (the relay addresses it from the claim)
upstream: static -> nat_ip/nat_prefix via nat_gw, all three required, else stop
          dhcp   -> udhcpc (its script writes the lease's resolv.conf); no
                    lease -> stop
iptables: redirect 80/443/53 from the build link; masquerade upstream; drop
          forwarding
resolv.conf: not written by the init itself
WARDEN_CLAIM_TOKEN=<token> relay --mode=vm --build-if=<build link interface>
stop: a console line (COM1 on Hyper-V), no relay; the claim times out
```

## Namespace holder (`cmd/warden-pause`)

```
warden-pause:
    wait for SIGTERM or SIGINT
    exit 0
```

It runs before the container driver's network is isolated, so it has no
network code, no shell, and no capabilities.
