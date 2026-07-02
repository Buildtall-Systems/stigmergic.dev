{
  description = "stigmergic.dev - A markdown file watcher and renderer";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
            templ
            tailwindcss
            nodejs
            goreleaser
          ];

          shellHook = ''
            echo "stigmergic.dev development environment"
            echo "Go version: $(go version)"
            echo "Templ version: $(templ version 2>/dev/null || echo 'installed')"
            echo "Tailwind: $(tailwindcss --help > /dev/null 2>&1 && echo 'installed' || echo 'not found')"
            echo ""
          '';
        };

        packages.default = pkgs.buildGoModule rec {
          pname = "stigmergic";
          version = "0.3.1";
          src = ./.;
          vendorHash = null;

          ldflags = [ "-s" "-w" "-X main.version=${version}" ];

          nativeBuildInputs = [ pkgs.templ ];

          preBuild = ''
            templ generate
          '';
        };
      }
    );
}
