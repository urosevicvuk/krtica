{
  description = "krtica — self-hosted, k8s-native reverse tunnel (Serbian: mole)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go # compiler + the `go` CLI (build/test/vet/mod/...)
            gopls # LSP server for your editor
            gotools # goimports, stringer, etc.
            go-tools # staticcheck
            golangci-lint # meta-linter, CI gate (roadmap Phase 0)
            gofumpt # stricter gofmt (roadmap Phase 0)
            delve # debugger (dlv)
            protobuf # protoc — control-stream + control-API messages (Decision #18)
            protoc-gen-go # protobuf → Go codegen
            protoc-gen-go-grpc # gRPC service stubs (control API, Phase 3)
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt-rfc-style);
    };
}
