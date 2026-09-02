{
  description = "Ultra-fast, native-like SSH workspace MCP server for AI coding agents";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "mcp-ssh-workspace";
          version = "1.0.0";
          src = ./.;

          vendorHash = null;

          subPackages = [ "cmd/mcp-ssh-workspace" ];

          env = {
            CGO_ENABLED = 0;
          };

          meta = with pkgs.lib; {
            description = "Native-like SSH workspace MCP server for AI coding agents";
            homepage = "https://github.com/surtr85/mcp-ssh-workspace";
            license = licenses.mit;
            mainProgram = "mcp-ssh-workspace";
          };
        };
      });

      apps = forAllSystems (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.default}/bin/mcp-ssh-workspace";
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            golangci-lint
          ];
        };
      });
    };
}
