FROM golang:1.26-bookworm

ARG GOPROXY=direct
ARG GONOSUMCHECK=*
ARG GONOSUMDB=*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /out/warden ./cmd/warden/
RUN CGO_ENABLED=0 go build -o /out/relay ./cmd/relay/

RUN warden-io post /out/warden
RUN warden-io post /out/relay
