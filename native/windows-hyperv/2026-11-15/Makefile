# Scratch Makefile for backlog entry project-config-trust-hyperv
# (host:windows-hyperv/20261116T051002Z). main has no --config, so its build
# reads [vm] image_refresh_after from the working directory's warden.toml, and
# the native allowlist's dist/warden* form (run from the checkout root) cannot
# change directory.
SHELL := sh

.PHONY: main-refresh state

# main's dist/warden (no .exe) with a refresh due: does the detached prepare start?
main-refresh:
	@cat named/warden.toml
	cd named && ../../../dist/warden build --driver hyperv --guest-os windows -o ../out-main-refresh ../proj/build.ps1; echo "exit=$$?"

state:
	powershell -NoProfile -ExecutionPolicy Bypass -File state.ps1
