.PHONY: build test lint fmt tidy cover clean

build:
	go build -o warden ./cmd/warden/
	go build -o ledger-inspect ./cmd/ledger-inspect/

test:
	go test ./...

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
	rm -f warden ledger-inspect coverage.out
