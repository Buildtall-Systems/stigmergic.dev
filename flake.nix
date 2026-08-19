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
            export GOPRIVATE="github.com/buildtall-systems/buildtall"
            export GIT_CONFIG_COUNT=1
            export GIT_CONFIG_KEY_0="url.git@github.com:buildtall-systems/.insteadOf"
            export GIT_CONFIG_VALUE_0="https://github.com/buildtall-systems/"
          '';
        };

        packages.default = pkgs.buildGoModule rec {
          pname = "stigmergic";
          version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ./VERSION);
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
