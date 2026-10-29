FROM alpine:3.21
COPY one.txt /src/one.txt
COPY sub /src/sub
RUN cat /src/one.txt /src/sub/two.txt
RUN ls -la /opt/warden
RUN echo "CA-BUNDLE-CERTS=$(grep -c 'BEGIN CERTIFICATE' /etc/ssl/certs/ca-certificates.crt)"
RUN if wget -q -T 5 -O- http://1.1.1.1:8080/ ; then echo EGRESS-OPEN; exit 1; else echo EGRESS-BLOCKED; fi
RUN wget -q -O /tmp/idx.tgz https://dl-cdn.alpinelinux.org/alpine/v3.21/main/aarch64/APKINDEX.tar.gz && echo HTTPS-VIA-RELAY-OK
RUN apk add --no-cache jq && jq --version
RUN cat /src/one.txt > /tmp/out.txt && warden-io post /tmp/out.txt
