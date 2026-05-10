FROM golang:1.26-bookworm AS build

RUN apt-get update && \
    apt-get install -y --no-install-recommends curl && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
# GOPROXY=direct: proxy.golang.org is blocked on this corporate network.
# Remove this override if building from a network with open access.
RUN GOPROXY=direct GONOSUMCHECK=* GONOSUMDB=* go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /out/warden ./cmd/warden/
RUN CGO_ENABLED=0 go build -o /out/relay ./cmd/relay/

RUN curl -fsSL -X POST --data-binary @/out/warden "http://artifacts/warden"
RUN curl -fsSL -X POST --data-binary @/out/relay "http://artifacts/relay"
