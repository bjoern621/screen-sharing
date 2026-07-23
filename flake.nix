{
  description = "screen-sharing: high-quality group screen sharing (MediaMTX relay + Wails app)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        linuxDeps = with pkgs; [
          gtk3
          webkitgtk_4_1   # Wails v2 Linux backend (build with tag webkit2_41)
          pkg-config
        ];
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            nodejs_22
            wails
            ffmpeg-full   # includes x11grab/kmsgrab + ffplay
            mediamtx      # the relay, run natively: `mediamtx mediamtx.yml`
            go-task       # task runner, see Taskfile.yml
          ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux linuxDeps;

          shellHook = ''
            echo "screen-sharing dev shell - run 'task' for available commands"
          '';
        };
      });
}
