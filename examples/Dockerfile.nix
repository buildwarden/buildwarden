FROM nixos/nix

# Install jq
RUN nix-env -iA nixpkgs.jq
