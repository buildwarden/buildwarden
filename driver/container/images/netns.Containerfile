# The container driver's iptables sidecar. It runs --privileged in the holder's
# network namespace, applies the isolation rules and exits. Its base is pinned
# by digest; warden runs the published image by digest, never by tag.
FROM docker.io/library/alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f
RUN apk add --no-cache iptables iptables-legacy
ENTRYPOINT ["sh", "-c"]
