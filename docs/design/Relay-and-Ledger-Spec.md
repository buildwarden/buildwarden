# BuildWarden Relay

This is the heart of what gives BuildWarden the ability to give the guarantees on completeness it does from its ledger.  It is a fully encapsulated TLS-terminating and forwarding proxy similar to the python package `mitmproxy`, but narrowly scoped to minimize its exploitable surface area and resource usage while maximizing performance.

Initial implementation is intended to be Rust and should build to native standalone binary executables for specific platforms, with initial targets of Linux on X86_64 and arm64 architectures.  These are then intended to be easily constructed/incorporated into light-weight VM images/containers and, if possible, minimal unikernel images.


## Startup Behavior

### Inputs and outputs

Any references to specific output files below are relative to the ledger's root directory as the self-encapsulated nature of the relay's binary should mean that the only potential inputs are:


1. Ledger output directory, which must not exist at start and may have a default based on OS/container
2. HTTP/HTTPS ports exposed, which by default are the default HTTP/HTTPS ports of 80/443
3. Hash methods used for object identifiers in the ledger.  All hashes are computed for every entry and are also used as part of signature generation in specified order, the default set consisting of `blake2b_256`, `sha256`, and `md5`.

    * These are intended for usage in multiple ways and so are in formats commonly in package repositories of multiple types.

1. HTTP/HTTPS Proxy for forwarding external requests, defaulting to no proxy.  This is intentional to allow for the usage of internal repository mirrors that may filter usage or availability more aggressively based on licensing or regulations, so that both open source and closed source ecosystems can be built/tracked using the same content-identifiable tracking ledgers.

### The Root Certificate

When the relay is first started, it should generate its ephemeral Certificate-Authorizing x509 certificate.  An example of this is in `src/certificate.rs`.  **The private portion of this certificate should never be exposed outside of the relay and should only be used for signing purposes within the relay itself**.  This certificate serves 2 purposes:


1. It is the Root Certificate-Authorizing certificate that will be used to generate site-specific certificates, allowing the relay to validate requests as if it is an authoritative endpoint for any external entity.
    1. These site-specific certificates are cached within the relay and while the private portion should not be disclosed as a matter of course, it also isn't sensitive as none of the ledger outputs depend on it.
    2. These certificates are what allow it to fully unencrypt the traffic prior to resubmitting the request externally which means the resources transferred are transparent from the build-hosts' perspective
2. It is used to sign all entries in the ledger as a verification that it is a part of this ledger specifically and that the records haven't been tampered with

After generation, it should write the public portion of the root certificate to `ledger.cert.<format>` in the PEM and DER file formats.  While other operating systems and languages may have other formats they prefer, the relay is not responsible for the conversion or the system/language-specific installation of the certificates as that is in the domain of the top-level orchestrator which starts or uses a pre-initiated relay and sets up the isolated build environment.


## Network Behavior

The relay is an opaque proxy that has the following server interfaces:


* `https://ledger/[<path>]`: The only reserved endpoint
    * `GET` requests expose the ledger output directory, containing the `ledger`, public certificates, `metadata`, `payloads`, and `artifacts` — which are just thin links to their associated `payloads`.  For directories, this lists the contents of that directory and for files will retrieve specified file.  For this reason, all `payloads` stored are not recorded by their retrieval metadata (such as url or filename), but instead based on their signature in the top level `ledger` metadata file. 
    * `POST` requests are recorded to the `ledger` in the same way all external requests (i.e. `payload` and signature) except:
        * It is not forwarded as a request to any external system by default
        * It is given a metadata schema of `artifact` with a  `<path>` entry as its only metadata
    * `PUT` requests require `<path>` and are used for appending metadata records to that path
* Acts as a MITM for HEAD/GET/OPTIONS HTTP/HTTPS requests with the exception of the reserved `ledger` hostname, used for communicating with the relay directly
    * Allows direct forwarding for HEAD/GET/OPTIONS methods once the request details are recorded to the ledger
    * Refuses all other HTTP methods except for the special cases below
    * Special cases:
        * A one-time-accepted POST request to `https://build-warden-environment` with initialization details about the build environment.
        * POST requests to `https://artifacts/<artifact_name>` are **not** forwarded externally, and are written to `artifacts/<signature>` once the signature has been determined.  This is for posting artifacts intended for visibility outside of the build jail.  This can be everything from the actual package artifact, docs, coverages results, and anything to be exposed from inside the build jail for outside consumption.
        * PUT requests to `https://metadata/<stream>` are **not** forwarded, but after having their ledger entry committed are then appended to `metadata/<stream>` and is new-line terminated.  This is for metadata that is output directly by build systems like Cargo to have an append-only stream to indicate metadata they would like to report about dependencies directly known as being included in potential outputs
* Implements standard HTTPS Proxy **following** for the forwarded requests
    * This allows for separation of potential caching, resurfacing of prior artifacts, mirrors, and credentialed/authenticated repository endpoints
* (Up for Design discussion) Acts as a DNS and/or DHCP server to forward all traffic to itself
    * This does require the Relay understand how it will be addressed within the network of the build environment
    * There may be a simpler way for making sure a jailed host attempts to forward all external traffic through the relay exclusively, but this is what I've come up with so far

## Ledger Format

The file itself exists at `ledger` which consists of a header with metadata about formatting and expectations and newline-delimited entries


### Header

The header is composed of multiple pieces and not strictly designed in format:


* Ledger schema/version: For now, `1.0`
* Entry format: default `json`, but may be switched to a binary format for support later
* List of hashes: Ordering of hash computation for signature generation.  Should include common hash types used by packages repositories
* `blake2b_256`, `sha256`, and `md5`
* Environment: One of the following types:
    * `container: <container_digest identity> (+ metadata)`
        * This is the specific container by its digest identity and will get forwarded from the orchestrator.
        * This will be the initial plan for the prototype which will simply take the reference at the top of the input Dockerfile
    * `vm: <hash-identity> (+ metadata)`:  At least for the short term, this will be necessary to support OSX/Windows build processes unless a similar container layer is put together
    * `ledgerized`:  This is more precise, but has enough edge-cases around how to do itin a way that is both complete and securely-external to the underlying build process itself.  It would include:
        * Any non-filesystem environment variables or configurations that cannot be individually monitored
        * Individual files read are measured either by the filesystem module/interface itself or a supervising stracing process.
* Signature of the header's contents
    * Taking a deterministic serialization of the header 's contents, create a signature using the root certificate

#### Up for design discussion

For the initial prototype, the environment will be container information and may be either a part of instantiation or could be its own entry category that is submitted secondarily to the orchestrator or supervising processes (which requires network isolation or authentication respectively).  If it's used as an entry, it's definitely a special one as unlike the other entities, its metadata is much more strictly controlled and is making guarantees on completeness as a result.



### Entries

In order to facilitate asynchronous concurrency within build processes, entries are categorized into the following types

* `open`
    * This represents a reference for opening of a channel that requires a closing entry before being considered complete
    * Examples of this include a new HTTP request (in either direction) or opening of a file-handle of an pre-existing environment file
    * This is likely to include `metadata` related to the specific resource, such as the filename, url, HTTP request type, and any pass-through http headers
    * This is may include an `out-payload` for relevant pass-through HTTP headers for GET requests if they are too big to reliably fit into the ledger's metadata
* `close`
    * This represents the closing of an existing `open` entry.  Any entries referencing the `open` entry afterwards are considered an invalid ledger
    * Must reference the `open` entry's signature
    * This likely includes the `payload` for reads/responses for artifacts/metadata when too large
* `checkpoint`
    * This represents a meaningful segmentation of data transfer related to an `open` entry without closing it
    * Must reference the `open` entry's signature
    * This may be used for the HTTP headers passed through to the client if they are sufficiently large that they aren't representable in the `close` entry's metadata

All entries in the ledger have the following structure:

* `entry_type`: `open|checkpoint|close`
* `open_signature` (Required for `checkpoint` and `close` entry types): Signature of the associated `open` request
* `payload`:
    * Omitted if no externally-identifiable data blob is transmitted that isn't covered in `metadata`
    * Payload contents are recorded to `payloads/<signature>`
    * `direction`: `in|out`
    * `size`: bytes, in 64-bit unsigned integer
    * `hashes`: For each hash in the top-level header, the computed digest.  Format up for discussion (could be in-order of ledger definition or key-value)
* `signature`: Calculated signature calculated by concatenating the following inputs and signing them with the ledger's private key
    * Previous entry's `signature`
    * If `payload` exists:
        * `size`
        * `hashes` in order specified in the ledger's header
* `metadata`: A generically serializable `{str: any}` store for storing informational metadata about the entry:
    * Because no system is necessarily authoritative, this information is not considered as part of the matching criteria of payloads as that is only authoritatively identifiable based on its content.  As such, this data is only intended to help give pointers to the potential relationships or request information for later analyzers to potentially consider.
    * The only controlled field is `**schema**` which will later indicate metadata field definitions and expectations.
        * TODO: This is not intended for the initial prototype as the field definitions are likely to change rapidly as the prototype solves for different challenges, so the metadata fields are considered only defined in implementation for now.

## Ledger Analysis

From an analysis standpoint, any entry made is only considered to have a complete historical record once the entry itself is closed and all prior open entries are closed.  As such, the ledger can be truncated encompass the complete history of one or more specific `payload:out` records without losing verifiability.  This is useful for serialized build processes where non-transitive outputs (testing/coverage information, documentation) are placed after building the initial artifact and so while that record may be useful for developers and troubleshooting, it can be truncated to the build artifact alone when publishing and sharing with consumers.

