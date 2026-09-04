# The Discord manager, as the machine beside the relay installs it (docs/discord-mode.md).
#
# A package of its own for the reason groupd.nix is one:
# it lands on the relay's machine, and nothing a server serving JSON needs rides in the app's closure.
{
  lib,
  buildGoModule,
  version,
}:

buildGoModule {
  pname = "screenshare-discordd";
  inherit version;

  # The Go tree alone, groupd.nix's filter verbatim: nothing here builds the shell.
  src = lib.cleanSourceWith {
    name = "screenshare-discordd-source";
    src = ../../.;
    filter =
      path: _:
      let
        rel = lib.removePrefix "${toString ../../.}/" (toString path);
        top = lib.head (lib.splitString "/" rel);
      in
      lib.elem top [
        "api"
        "backend"
      ];
  };

  # The module cache rather than a vendor directory, for the reason packaging/nix/package.nix states.
  # The hash is the module's one dependency set, shared with the other two binaries.
  proxyVendor = true;
  vendorHash = "sha256-3sMf233ihFSRiN1kr+uG3lljk0XUDrhkTbrGRe5Z1Bs=";

  modRoot = "backend";
  subPackages = [ "cmd/discordd" ];

  # Nothing this binary imports reaches backend/internal/receive,
  # the one cgo and GStreamer package,
  # so it builds without a C compiler or GStreamer headers on the server.
  env.CGO_ENABLED = 0;

  meta = {
    description = "Discord manager for the screen-sharing relay: voice channels as groups";
    mainProgram = "discordd";
    platforms = lib.platforms.linux;
  };
}
