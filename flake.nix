{
  description = "mocrelay - A Nostr relay implementation in Go";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            lefthook
          ];

          # encoding/json/v2 を有効化
          GOEXPERIMENT = "jsonv2";

          shellHook = ''
            echo "mocrelay dev environment loaded"
            echo "GOEXPERIMENT=$GOEXPERIMENT"
          '';
        };
      }
    );
}
