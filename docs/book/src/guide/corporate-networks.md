# Corporate Networks

Some corporate networks filter or proxy specific traffic. This page covers common workarounds.

## Go Module Proxy

If your network blocks `proxy.golang.org`, set `GOPROXY=direct` in your Dockerfile to download modules directly from source repositories:

```dockerfile
RUN GOPROXY=direct GONOSUMCHECK=* GONOSUMDB=* go mod download
```

If your organization runs an internal Go module proxy:

```dockerfile
ENV GOPROXY=https://goproxy.corp.example.com
RUN go mod download
```

## Internal Container Registries

If your base images come from an internal registry, they'll work normally — BuildWarden intercepts all HTTPS traffic regardless of hostname:

```dockerfile
FROM registry.corp.example.com/base/golang:1.26
```

The relay will MITM the TLS connection to your internal registry and record all layer pulls in the ledger.

## HTTP Proxies

BuildWarden doesn't currently support explicit forward proxy (`http_proxy`/`https_proxy`) mode. All traffic is intercepted transparently via iptables DNAT. If your network requires an explicit proxy to reach the internet, the relay handles this — it has unrestricted outbound access and forwards traffic upstream.

## DNS Filtering

The relay forwards DNS queries to the upstream resolver configured in the container runtime's network. If certain domains are blocked at the DNS level by your corporate resolver, those blocks will be reflected in the build (same as building without BuildWarden).

## Certificate Trust

The relay generates an ephemeral CA per build and injects it into the build container's trust store. Tools that use the system certificate store (apt, pip, curl, most language package managers) will trust the relay automatically.

Tools with their own certificate bundles may need explicit configuration via extensions. BuildWarden includes extensions for pip and Bazel. See [Extensions](../concepts/extensions.md) for adding support for other tools.
