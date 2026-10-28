# The container driver's network-namespace holder (cmd/warden-pause). It runs
# before the build's network is isolated, so it carries one static binary that
# does nothing: no shell, no network code, nothing listening.
# Context: warden-pause-<arch> binaries (GOOS=linux CGO_ENABLED=0).
FROM scratch
ARG TARGETARCH
COPY --chmod=0755 warden-pause-${TARGETARCH} /pause
USER 65534:65534
ENTRYPOINT ["/pause"]
