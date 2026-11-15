# Scratch Makefile for backlog entry runtime-cli-project-path-windows
# (host:container-any/20261115T231002Z). warden reads the project's warden.toml
# from the working directory and has no flag that names it, so each case has to
# run from its project directory, which the native allowlist's
# .scratch/<name>/dist/warden* form (run from the checkout root) cannot do.
#
# W is the warden under test: the branch build by default, main's with
# W=../../../dist/warden.
W ?= ../../rcpp/dist/warden
SHELL := sh

.PHONY: env exe path bat docker podman podman-clean

# run <dir> <args>: the command, its output and its exit status.
define run
	@echo "--- (cd $(1) && $(W) $(2))"; cd $(1) && $(W) $(2); echo "exit=$$?"
endef

env:
	powershell -NoProfile -ExecutionPolicy Bypass -File env.ps1

# --runtime docker.exe, with no warden.toml in the directory.
exe:
	$(call run,p-exe,doctor --driver container --runtime docker.exe)
	$(call run,p-exe,build --driver container --runtime docker.exe -o out)
	$(call run,p-exe,clean --driver container --runtime docker.exe)
	$(call run,p-exe,build --driver container --runtime Docker.EXE -o out)
	@ls p-exe

# [runtime] cli = the full path of docker.exe, in the project's warden.toml.
path:
	@cat p-path/warden.toml
	$(call run,p-path,doctor --driver container)
	$(call run,p-path,build --driver container -o out)
	$(call run,p-path,clean --driver container)
	@ls p-path

# [runtime] cli = a .bat the directory ships, in three spellings.
bat:
	@rm -f p-bat/rt-ran.txt
	@for t in bat-name bat-dot bat-abs; do \
		cp toml/$$t.toml p-bat/warden.toml; echo "=== $$t"; cat p-bat/warden.toml; \
		for a in "doctor --driver container" "build --driver container -o out" "clean --driver container" "clean" "doctor"; do \
			echo "--- (cd p-bat && $(W) $$a)"; (cd p-bat && $(W) $$a); echo "exit=$$?"; \
		done; \
	done
	@ls p-bat
	@if [ -e p-bat/rt-ran.txt ]; then echo "MARKER PRESENT: rt.bat was run"; cat p-bat/rt-ran.txt; else echo "marker absent: rt.bat was not run"; fi

# cli = "docker" in the project's warden.toml.
docker:
	@cat p-docker/warden.toml
	$(call run,p-docker,doctor --driver container)
	$(call run,p-docker,build --driver container -o out)

# cli = "podman" in the project's warden.toml (this host's runtime).
podman:
	@cat p-podman/warden.toml
	$(call run,p-podman,doctor --driver container)
	$(call run,p-podman,build --driver container -o out)
	$(call run,p-podman,inspect out)

podman-clean:
	$(call run,p-podman,clean --driver container)
