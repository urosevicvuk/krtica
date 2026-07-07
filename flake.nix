{
  description = "krtica - self-hosted, k8s-native reverse tunnel (Serbian: mole)";

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
            go 
            gopls 
            gotools 
            go-tools 
            golangci-lint 
            gofumpt 

            delve 

            protobuf 
            protols 
            protoc-gen-go 
            protoc-gen-go-grpc 

            go-task 
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt-rfc-style);
    };
}
