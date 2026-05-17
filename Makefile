.PHONY: build test integration-test lint fmt tidy cover clean sign relay-vm

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

build:
	go build -o warden ./cmd/warden/
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o warden-relay-linux-arm64 ./cmd/relay/
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o warden-io-linux-arm64 ./cmd/warden-io/
ifeq ($(UNAME_S),Darwin)
ifeq ($(UNAME_M),arm64)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o warden-io-darwin-arm64 ./cmd/warden-io/
	@$(MAKE) sign
endif
endif

sign:
ifeq ($(UNAME_S),Darwin)
	codesign --force --sign - --entitlements entitlements.plist ./warden
	@echo "Signed warden with virtualization entitlement (ad-hoc)"
endif

test:
	go test ./...

integration-test:
	./test-integration.sh

integration-test-vz:
ifeq ($(UNAME_S),Darwin)
	go test -c -tags integration -o vz-test ./driver/vz/
	codesign --force --sign - --entitlements entitlements.plist ./vz-test
	./vz-test -test.v -test.run TestRelayVMBoot -test.timeout 60s
	@rm -f vz-test
else
	@echo "VZ integration tests require macOS/arm64"
endif

relay-vm:
	./tools/relay-vm/build-initramfs.sh

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
	rm -f warden warden-relay-linux-arm64 warden-io-linux-arm64 \
		warden-io-darwin-arm64 vz-test coverage.out
