{
  description = "Host-aware WezTerm spawn actions and wezspawn CLI";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  outputs =
    { self, nixpkgs }:
    let
      each = nixpkgs.lib.genAttrs [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
    in
    {
      packages = each (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          wezspawn = pkgs.buildGoModule {
            pname = "wezspawn";
            version = "0.1.0";
            src = pkgs.lib.fileset.toSource {
              root = ./.;
              fileset = pkgs.lib.fileset.unions [
                ./go.mod
                ./cmd
                ./internal
              ];
            };
            vendorHash = null;
            subPackages = [ "cmd/wezspawn" ];
            checkPhase = "go test ./...";
            meta.mainProgram = "wezspawn";
          };
          default = self.packages.${system}.wezspawn;
        }
      );
      checks = each (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          wezspawn = self.packages.${system}.wezspawn;
          plugin = pkgs.runCommand "spawn-plugin-tests" { nativeBuildInputs = [ pkgs.lua5_4 ]; } ''
            lua ${./tests/plugin.lua} ${./plugin/init.lua}
            touch $out
          '';
        }
      );
      devShells = each (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.lua5_4
              pkgs.stylua
              pkgs.nixfmt
              pkgs.wezterm
            ];
          };
        }
      );
      formatter = each (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
