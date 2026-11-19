# Scratch Makefile for detached-prepare-windows-exe f6-validate-windows-hyperv
# (host:windows-hyperv/windows-desktop.20261119T171002Z). The native allowlist
# runs make -C .scratch/<name>, not go or powershell directly.
SHELL := sh
B := ../dpwe
W := $(B)/dist/warden
CFG := $(CURDIR)/cfg/warden.toml

.PHONY: test doctor build refresh refresh-exe state hold-log mkexe rmexe

# Case 1: tier-0 cases on the Windows host.
test:
	cd $(B) && go version && go test -count=1 -v -run 'Detach|Detached|Refresh|Fresh|StartDetached' ./driver/ ./driver/hyperv/ ./driver/qemu/ 2>&1 | tail -80
	cd $(B) && go test -count=1 ./driver/ ./driver/hyperv/ ./driver/qemu/

doctor:
	$(W) doctor --driver hyperv; echo "exit=$$?"

# A refresh-due build (cases 2-5), with the warden named by W.
refresh:
	@cat $(CFG)
	$(W) build --config $(CFG) --driver hyperv --guest-os windows -o out-$(TAG) proj/build.ps1; echo "exit=$$?"

state:
	powershell -NoProfile -ExecutionPolicy Bypass -File state.ps1

# Case 4: hold prepare.log open with no sharing for HOLD seconds.
hold-log:
	powershell -NoProfile -ExecutionPolicy Bypass -File hold.ps1 -Seconds $(HOLD)

# Case 3 and 5: a warden.exe beside dist/warden.
mkexe:
	cd $(B) && go build -o dist/warden.exe ./cmd/warden && ls -l dist/

rmexe:
	rm -f $(B)/dist/warden.exe && ls -l $(B)/dist/
