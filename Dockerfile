FROM golang:1.26-bookworm AS build

RUN apt-get update && \
    apt-get install -y --no-install-recommends curl git && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY=direct GONOSUMCHECK=* GONOSUMDB=* GOFLAGS=-insecure go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /out/warden ./cmd/warden/

RUN curl -fsSL -X POST --data-binary @/out/warden "http://artifacts/warden"
