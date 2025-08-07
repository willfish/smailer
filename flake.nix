{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { system = system; };

        lint = pkgs.writeScriptBin "lint" ''
          pre-commit run --all-files --show-diff-on-failure
        '';
        smailer = pkgs.buildGoModule {
          pname = "smailer";
          version = "0.1.0";

          src = ./.;

          vendorHash = "sha256-Z/p1FavHvLQ91UPgIKqtJuuu4zjJ3GRKACdHtQLV5f4=";

          meta = with pkgs.lib; {
            description = "A tool for reviewing emails in s3 forwarded by SES";
            homepage = "https://github.com/willfish/smailer";
            license = licenses.mit;
            maintainers = [ maintainers.willfish ];
          };
        };
      in
      {
        packages.default = smailer;
        packages.smailer = smailer;
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            golangci-lint
            gopls
            lint
          ];
        };
      }
    );
}
