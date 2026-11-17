# runtime-cli-project-path-finch (macos-finch), host:macos-finch/20261117T124004Z, main 0cd3c02.
# Run from the checkout root as: make -C .scratch/rcp-finch <target>
W = $(CURDIR)/../main/dist/warden
FINCH = /usr/local/bin/finch

# A build with cli = "finch" in the project's warden.toml.
build:
	cd case && $(W) doctor --driver container; echo "exit=$$?"
	cd case && $(W) build --driver container --output $(CURDIR)/out-build .; echo "exit=$$?"
	cd case && $(W) inspect $(CURDIR)/out-build; echo "exit=$$?"

# --runtime with the full path of finch, on build and on clean --driver vz.
refuse:
	$(W) build --driver container --runtime $(FINCH) --output $(CURDIR)/out-refuse case; echo "exit=$$?"
	ls -d $(CURDIR)/out-refuse 2>&1 || true
	$(W) clean --driver vz --runtime $(FINCH); echo "exit=$$?"
