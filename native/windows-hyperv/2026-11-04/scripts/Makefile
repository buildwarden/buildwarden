# Scratch launcher for the branch's relay boot VHDX refresh (the native
# allowlist runs make and .scratch/*/dist/warden*, not .scratch/*/tools/*).
refresh:
	cd ../build-link-subnet && tools/relay-vm/hyperv/refresh-cache.sh

check:
	cd ../build-link-subnet && tools/relay-vm/hyperv/refresh-cache.sh --check

# The old (pre-change) VHDX under a forged link-channel stamp: the old relay
# refuses the claim's build_link, which warden maps to the refresh fix.
forged:
	cd ../.. && WARDEN_RELAY_BOOT_VHDX=C:/Users/jeff/buildwarden-native/.scratch/hv/old/relay-boot.vhdx \
		.scratch/build-link-subnet/dist/warden.exe build --driver hyperv --guest-os windows \
		--script .scratch/hv/ctx/build.ps1 --timeout 5m -o .scratch/hv/out-forged .scratch/hv/ctx
