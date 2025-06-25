{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, flake-utils, nixpkgs }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
        {
          devShell = pkgs.mkShell {
            buildInputs = with pkgs; [ go ];
          };
          packages.default = pkgs.buildGoModule {
            pname = "environ";
            version = "0.1.0";
            src = ./.;
            vendorHash = "sha256-7CsvNC6Fr8z4Rz5pULFscQU84T7hF84V1nJEKIYa7y8=";
          };
        }
    );
}