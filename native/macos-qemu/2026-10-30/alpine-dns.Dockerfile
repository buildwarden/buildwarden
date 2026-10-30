FROM alpine:3.21
RUN apk add --no-cache jq && jq --version > /tmp/jq.txt && warden-io post /tmp/jq.txt
