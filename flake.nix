{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    pre-commit-hooks = {
      url = "github:cachix/git-hooks.nix";
    };
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      pre-commit-hooks,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };

        lint = pkgs.writeScriptBin "lint" ''
          pre-commit run --all-files --config .pre-commit-config-nix.yaml --show-diff-on-failure
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

        preCommitCheck = pre-commit-hooks.lib.${system}.run {
          src = ./.;
          configPath = ".pre-commit-config-nix.yaml";
          default_stages = [ "pre-commit" ];
          hooks = {
            actionlint = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-added-large-files = {
              enable = true;
              args = [ "--maxkb=1000" ];
              stages = [ "pre-commit" ];
            };
            check-case-conflicts = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-executables-have-shebangs = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-json = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-merge-conflicts = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-shebang-scripts-are-executable = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-toml = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            check-yaml = {
              enable = true;
              args = [ "--allow-multiple-documents" ];
              stages = [ "pre-commit" ];
            };
            deadnix = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            detect-private-keys = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            end-of-file-fixer = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            golangci-lint = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            mixed-line-endings = {
              enable = true;
              args = [ "--fix=lf" ];
              stages = [ "pre-commit" ];
            };
            nixfmt-rfc-style = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            shellcheck = {
              enable = true;
              stages = [ "pre-commit" ];
            };
            statix = {
              enable = true;
              settings.ignore = [ ".direnv" ];
              stages = [ "pre-commit" ];
            };
            trim-trailing-whitespace = {
              enable = true;
              stages = [ "pre-commit" ];
            };
          };
        };
      in
      {
        packages.default = smailer;
        packages.smailer = smailer;
        devShells.default = pkgs.mkShell {
          shellHook = ''
            ${preCommitCheck.shellHook}
          '';

          buildInputs =
            preCommitCheck.enabledPackages
            ++ (with pkgs; [
              go
              golangci-lint
              gopls
              lint
            ]);
        };
      }
    );
}
