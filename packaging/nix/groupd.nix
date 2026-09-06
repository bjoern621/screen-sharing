# The key, token and index service, as the machine beside the relay installs it.
#
# A package of its own rather than a binary inside the app's, the two landing on different machines:
# the app package carries a desktop entry, a GStreamer plugin path and an ffmpeg closure,
# none of which a server serving JSON has a use for.
{
  lib,
  buildGoModule,
  version,
}:

buildGoModule {
  pname = "screenshare-groupd";
  inherit version;

  # The Go tree alone, packaging/nix/package.nix's filter without avalonia/: nothing here builds the shell.
  src = lib.cleanSourceWith {
    name = "screenshare-groupd-source";
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

  # The module cache rather than a vendor directory, for the reason packaging/nix/package.nix states:
  # `api` is a module of this repository reached by a filesystem replace,
  # and vendoring would fold it into the hash that pins third-party code.
  # The hash is that package's, both builds downloading what one backend/go.mod lists.
  proxyVendor = true;
  vendorHash = "sha256-3sMf233ihFSRiN1kr+uG3lljk0XUDrhkTbrGRe5Z1Bs=";

  modRoot = "backend";
  subPackages = [ "cmd/groupd" ];

  # The number every answer names, which is how a member's relay check reads what a deployment
  # is running (backend/internal/groupsvc, version.go).
  ldflags = [ "-X main.version=${version}" ];

  # Nothing this binary imports reaches backend/internal/receive,
  # the one cgo and GStreamer package,
  # so it builds without a C compiler or GStreamer headers on the server.
  env.CGO_ENABLED = 0;

  meta = {
    description = "Group key, token and index service for the screen-sharing relay";
    mainProgram = "groupd";
    platforms = lib.platforms.linux;
  };
}
