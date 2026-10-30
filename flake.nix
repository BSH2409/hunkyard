{
  description = "Browser-based markdown review tool with inline commenting";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      version = "1.0.0";
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in rec {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          hunkyard = pkgs.buildGo126Module {
            pname = "hunkyard";
            inherit version;
            src = self;
            subPackages = [ "cmd/hunkyard" ];
            vendorHash = "sha256-X0OqstDm8a1yF2NXKTyFOvv+xbQgjvv/wYqRAUGm3tw=";
            # Tests run in dedicated CI jobs (test + e2e); the Nix sandbox's
            # /build TMPDIR cleanup races with the debounced review file writer.
            doCheck = false;
            ldflags = [ "-s" "-w" "-X main.version=${version}" ];
            meta = with nixpkgs.lib; {
              description = "Browser-based markdown review tool with inline commenting";
              homepage = "https://github.com/BSH2409/hunkyard";
              license = licenses.mit;
              mainProgram = "hunkyard";
            };
          };
        in {
          inherit hunkyard;
          default = hunkyard;
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${packages.${system}.default}/bin/hunkyard";
        };
      });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_26
              gopls
              golangci-lint
              git
            ];
          };
        });
    };
}
