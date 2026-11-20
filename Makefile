.PHONY: build test release-build integration-test driver-conformance lint tier1 enforcement fmt tidy cover clean sign relay-vm deadcode pin-specs conformance

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

build:
	@mkdir -p dist
	go build -o dist/warden ./cmd/warden/
	go build -o dist/warden-driver-container ./cmd/warden-driver-container/
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

# What a release compiles (.goreleaser.yaml, its hook, release.yml): each
# program, for each target, without cgo. Nothing is kept. A target or
# program added there is added here.
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
release-build:
	@for t in $(RELEASE_TARGETS); do \
		echo "== $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 \
			go build ./cmd/warden ./cmd/warden-driver-container ./cmd/warden-io \
				./cmd/relay ./cmd/warden-pause || exit 1; \
	done

integration-test:
	./test-integration.sh

# The driver contract's conformance suite, both tiers, against the driver
# binary DRIVER. The lifecycle tier runs real builds, so it needs a host the
# driver runs on; DRIVER_CONFORMANCE_FLAGS are driver-conformance's flags for
# that host (--driver-arg, --config-file, --base-image, --skip, ...).
DRIVER ?= dist/warden-driver-container
driver-conformance:
	go run ./conformance/driver/cmd/driver-conformance --driver $(DRIVER) \
		$(DRIVER_CONFORMANCE_FLAGS)

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

# FLOW-14: when the branch changes tools/tier1/ or tools/workflow/check/, run
# tier1 built from the merge base's code with the branch's rules.yaml and
# golangci.yml against it too, and fail on a finding the
# branch's tier1 drops without an approved rule change naming its rule.
enforcement:
	tools/workflow/enforcement-check.sh $(TIER1_BASE) $(TIER1_FLAGS)

# Code unreachable from any main or test, per platform (FLOW-15). Remove what
# all three report, or say why it stays.
DEADCODE := golang.org/x/tools/cmd/deadcode@v0.38.0
# go install pkg@version runs outside this module and would build with the Go
# on PATH; pin it to the module's toolchain, which deadcode needs to load the
# packages, and reinstall when dist/deadcode was built with another.
deadcode:
	@want=$$(go env GOVERSION); \
	if [ "$$(go version $(CURDIR)/dist/deadcode 2>/dev/null | cut -d' ' -f2)" != "$$want" ]; then \
		GOTOOLCHAIN=$$want GOBIN=$(CURDIR)/dist go install $(DEADCODE); \
	fi
	@for os in linux darwin windows; do \
		echo "== $$os"; GOOS=$$os $(CURDIR)/dist/deadcode -test ./... ; \
	done

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

# Moves the pins of the seam specifications, tools/conformance/<directory>/
# (Decision 7 of docs/design/seam-specs.md). Give SPECS="<directory>@<version>
# ...", SPECS=latest or REF=<commit>; it has no default. A pin is moved by this
# target and never by go get, which keeps the old pin's harness version.
pin-specs:
	SPECS='$(SPECS)' REF='$(REF)' sh tools/conformance/pin-specs.sh

# The seam specifications' suites against the relay and the disk collector
# built from this checkout (docs/design/seam-specs.md, The harness). Each
# suite's command is built from the release its own module
# tools/conformance/<directory>/ pins, with that release's harness, and runs
# against each target in tools/conformance/targets/; one table is printed per
# target. With TARGETS unset it runs every target whose file declares this
# host's OS and prints each one it leaves out; a named target that cannot run
# here is an error. SUITES names the specifications whose suites run (all six
# by default), SPECS_DIR=<checkout> builds the suites from a checkout of the
# specs repository instead of the pins, and OCI_RUNTIME runs relay-oci with
# that runtime. The relay targets need Linux and root or user namespaces, and
# relay-oci root and its runtime.
conformance:
	@TARGETS='$(TARGETS)' SUITES='$(SUITES)' SPECS_DIR='$(SPECS_DIR)' OCI_RUNTIME='$(OCI_RUNTIME)' \
		sh tools/conformance/conformance.sh

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -rf dist/ vz-test coverage.out
