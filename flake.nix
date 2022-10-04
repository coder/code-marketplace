{
  description = "Code extension marketplace";

  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem
      (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          devShells.default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go_1_19
              golangci-lint
              gotestsum
              kubernetes-helm
            ];
          };
        }
      );
}
