# Scratch Makefile for backlog entry project-config-trust-windows
# (host:container-any/20261116T051002Z). The cases need a working directory
# per project, a USERPROFILE that is not the machine's own (so no file is
# written under the real home) and WARDEN_* variables beside it, which the
# native allowlist's .scratch/<name>/dist/warden* form (run from the checkout
# root) cannot give.
#
# W is the warden under test: the branch build by default, main's with
# W=../../../dist/warden.
W ?= ../../pct/dist/warden
SHELL := sh
R := C:\Users\jeff\buildwarden-native\.scratch\pct-cases
UHOME := $(R)\home
UFILE := $(UHOME)\.config\warden\config.toml
NAMED := $(R)\named\warden.toml

.PHONY: env certs userfile userfile-cleared named named-build ca-doctor ca-build nul base-build clean-builds

# run <dir> <env> <args>: the command, its output and its exit status.
define run
	@echo "--- (cd $(1) && $(2) $(W) $(3))"; cd $(1) && $(2) $(W) $(3); echo "exit=$$?"
endef

env:
	powershell -NoProfile -ExecutionPolicy Bypass -File env.ps1

certs:
	mkdir -p ca
	openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj '//CN=pct-case-a' -keyout ca/a.key -out ca/a.pem
	openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj '//CN=pct-case-b' -keyout ca/b.key -out ca/b.pem
	rm -f ca/a.key ca/b.key
	ls -l ca

# The former user-level file present under USERPROFILE: every command exits 1
# naming it.
userfile:
	@cat home/.config/warden/config.toml
	$(call run,p-user,USERPROFILE='$(UHOME)',doctor)
	$(call run,p-user,USERPROFILE='$(UHOME)',doctor --driver container)
	$(call run,p-user,USERPROFILE='$(UHOME)',doctor --driver hyperv)
	$(call run,p-user,USERPROFILE='$(UHOME)',build --driver container -o out)
	$(call run,p-user,USERPROFILE='$(UHOME)',clean --driver container)
	@ls -A p-user

# WARDEN_CONFIG (and --config) naming that file clears the error; naming
# another file does not.
userfile-cleared:
	$(call run,p-user,USERPROFILE='$(UHOME)' WARDEN_CONFIG='$(UFILE)',doctor --driver hyperv)
	$(call run,p-user,USERPROFILE='$(UHOME)',doctor --driver hyperv --config '$(UFILE)')
	$(call run,p-user,USERPROFILE='$(UHOME)' WARDEN_CONFIG='$(NAMED)',doctor --driver hyperv)

# --config with a drive-letter path is read and .\warden.toml is not.
named:
	@echo "p-named/warden.toml:"; cat p-named/warden.toml; echo "named/warden.toml:"; cat named/warden.toml
	$(call run,p-named,,doctor --driver container)
	$(call run,p-named,,doctor --driver container --config '$(NAMED)')
	$(call run,p-named,WARDEN_CONFIG='$(NAMED)',doctor --driver container)

named-build:
	$(call run,p-named,,build --driver container --config '$(NAMED)')
	@ls -A p-named

# WARDEN_UPSTREAM_CA_CERTS split on ';', each entry a drive-letter path.
ca-doctor:
	$(call run,p-ca,WARDEN_UPSTREAM_CA_CERTS='$(R)\ca\a.pem;$(R)\ca\b.pem',doctor --driver container)
	$(call run,p-ca,WARDEN_UPSTREAM_CA_CERTS='$(R)\ca\a.pem;;$(R)\ca\b.pem',doctor --driver container)

ca-build:
	@echo "--- (cd p-ca && WARDEN_UPSTREAM_CA_CERTS='$(R)\ca\a.pem;$(R)\ca\b.pem' $(W) build --driver container -o out 2>../ca-build.stderr)"; \
	cd p-ca && WARDEN_UPSTREAM_CA_CERTS='$(R)\ca\a.pem;$(R)\ca\b.pem' $(W) build --driver container -o out 2>../ca-build.stderr; echo "exit=$$?"
	@echo "=== stderr:"; cat ca-build.stderr
	$(call run,p-ca,,inspect out)

# --config NUL reads no settings: ./warden.toml names a driver warden lacks.
nul:
	@cat p-nul/warden.toml
	$(call run,p-nul,,doctor)
	$(call run,p-nul,,doctor --config NUL)
	$(call run,p-nul,,doctor --driver container --config NUL)
	$(call run,p-nul,WARDEN_CONFIG=NUL,doctor --driver container)

# Baseline: the same Dockerfile with no settings (main: W=../../../dist/warden).
base-build:
	$(call run,p-base,,build --driver container -o out)
	$(call run,p-base,,inspect out)

clean-builds:
	$(call run,p-base,,clean --driver container)
	rm -rf p-base/out p-ca/out p-named/from-named p-named/from-repo p-user/out
