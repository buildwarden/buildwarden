.PHONY: build
build:
	go install ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: fmt
fmt:
	gofmt -s -w .

.PHONY: lint
lint:
	golangci-lint run
