.PHONY: build test integration-test lint fmt tidy cover clean sign relay-vm deadcode

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

build:
	@mkdir -p dist
	go build -o dist/warden ./cmd/warden/
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/warden-relay-linux-amd64 ./cmd/relay/
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o dist/warden-relay-linux-arm64 ./cmd/relay/
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/warden-io-linux-amd64 ./cmd/warden-io/
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o dist/warden-io-linux-arm64 ./cmd/warden-io/
ifeq ($(UNAME_S),Darwin)
ifeq ($(UNAME_M),arm64)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o dist/warden-io-darwin-arm64 ./cmd/warden-io/
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o dist/warden-relay-darwin-arm64 ./cmd/relay/
	@$(MAKE) sign
endif
endif

# Use CODESIGN_IDENTITY=- for ad-hoc signing (contributors without a Developer ID).
CODESIGN_IDENTITY ?= -

sign:
ifeq ($(UNAME_S),Darwin)
	@if codesign --force --sign "$(CODESIGN_IDENTITY)" --entitlements entitlements.plist dist/warden 2>/dev/null; then \
		echo "Signed warden with virtualization entitlement (identity: $(CODESIGN_IDENTITY))"; \
	else \
		echo "WARNING: codesign failed; VZ driver will not work without entitlement"; \
	fi
endif

test:
	go test ./...
	cd tools/workflow && go test ./...

integration-test:
	./test-integration.sh

integration-test-vz:
ifeq ($(UNAME_S),Darwin)
	@mkdir -p dist
	go test -c -tags integration -o dist/vz-test ./driver/vz/
	codesign --force --sign "$(CODESIGN_IDENTITY)" --entitlements entitlements.plist dist/vz-test
	dist/vz-test -test.v -test.run TestRelayVMBoot -test.timeout 60s
	@rm -f dist/vz-test
else
	@echo "VZ integration tests require macOS/arm64"
endif

relay-vm:
	./tools/relay-vm/build-initramfs.sh

lint:
	golangci-lint run
	cd tools/workflow && golangci-lint run -c ../../.golangci.yml

# Code unreachable from any main or test, per platform (FLOW-15). Remove what
# all three report, or say why it stays.
DEADCODE := golang.org/x/tools/cmd/deadcode@v0.38.0
deadcode:
	@GOBIN=$(CURDIR)/dist go install $(DEADCODE)
	@for os in linux darwin windows; do \
		echo "== $$os"; GOOS=$$os $(CURDIR)/dist/deadcode -test ./... ; \
	done

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -rf dist/ vz-test coverage.out
