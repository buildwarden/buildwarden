FROM nixos/nix

RUN nix-env -iA nixpkgs.jq
