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
            version = "0.2.0";
            src = ./.;
            vendorHash = "sha256-CWAkfjgO8wmXfTPpwqBGcW2HEX62vOUfE+UCuzkZmbg=";
          };
        }
    );
}
