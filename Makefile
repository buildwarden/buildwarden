.PHONY: build test integration-test lint tier1 fmt tidy cover clean sign relay-vm deadcode

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
	cd tools/tier1 && go test ./...

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
	cd tools/tier1 && golangci-lint run -c ../../.golangci.yml
	@$(MAKE) --no-print-directory tier1

# Tier-1 static checks over the diff from TIER1_BASE to HEAD (LINT-02). Pass
# TIER1_FLAGS=-no-design for a change that needs no design (FLOW-01).
TIER1_BASE ?= origin/main
TIER1_FLAGS ?=
# tier1 is built for the host, whatever GOOS the caller set, and then run with
# that GOOS, so `GOOS=<os> make lint` checks <os>'s build-tagged files (FLOW-17).
HOSTEXE := $(if $(filter windows,$(shell go env GOHOSTOS)),.exe,)
tier1:
	@mkdir -p dist
	cd tools/tier1 && GOOS=$$(go env GOHOSTOS) GOARCH=$$(go env GOHOSTARCH) \
		go build -o ../../dist/tier1$(HOSTEXE) ./cmd/tier1
	./dist/tier1$(HOSTEXE) -base $(TIER1_BASE) $(TIER1_FLAGS)

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
