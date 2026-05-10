FROM golang:1.26-alpine AS build

RUN apk add --no-cache curl git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /out/warden ./cmd/warden/

RUN curl -fsSL -X POST --data-binary @/out/warden "http://artifacts/warden"
