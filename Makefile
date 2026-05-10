.PHONY: build test integration-test lint fmt tidy cover clean

build:
	go build -o warden ./cmd/warden/
	GOOS=linux CGO_ENABLED=0 go build -o warden-relay ./cmd/relay/

test:
	go test ./...

integration-test:
	./test-integration.sh

lint:
	golangci-lint run

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -f warden warden-relay coverage.out
