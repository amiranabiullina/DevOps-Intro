{
  description = "QuickNotes — reproducible build (Lab 11)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        quicknotes = pkgs.buildGoModule {
          pname = "quicknotes";
          version = "0.1.0";
          src = ./app;

          # app/go.mod has no requirements (no go.sum), so there is nothing to vendor:
          # nixpkgs rejects any hash here with "vendor folder is empty, please set 'vendorHash = null;'".
          vendorHash = null;

          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" ];
        };

        default = quicknotes;

        # Built by Nix alone (no Docker daemon). `created` defaults to the epoch, so the tarball is deterministic.
        docker = pkgs.dockerTools.buildImage {
          name = "quicknotes";
          tag = quicknotes.version;
          copyToRoot = [ quicknotes ];
          extraCommands = ''
            mkdir -p app data etc
            cp ${./app/seed.json} app/seed.json
            echo 'nonroot:x:65532:65532:nonroot:/home/nonroot:/sbin/nologin' > etc/passwd
            echo 'nonroot:x:65532:' > etc/group
            chmod 1777 data
          '';
          config = {
            Entrypoint = [ "/bin/quicknotes" ];
            ExposedPorts = { "8080/tcp" = { }; };
            User = "65532:65532";
            WorkingDir = "/app";
            Env = [ "DATA_PATH=/data/notes.json" "SEED_PATH=/app/seed.json" ];
          };
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls pkgs.golangci-lint ];
        };
      });
    };
}
