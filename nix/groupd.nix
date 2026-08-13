# The key, token and index service, as the machine beside the relay installs it.
#
# A package of its own rather than a binary inside the app's, because the two are
# installed on different machines: the app package carries a desktop entry, a GStreamer
# plugin path and an ffmpeg closure, none of which a server serving JSON has a use for.
# What they share is this repository, which is the point (cmd/groupd).
{
  lib,
  buildGoModule,
  version ? lib.fileContents ../VERSION,
}:

buildGoModule {
  pname = "screenshare-groupd";
  inherit version;

  # The Go tree alone. The app package filters the same way and states why; this one
  # drops avalonia/ as well, since nothing here builds the shell.
  src = lib.cleanSourceWith {
    name = "screenshare-groupd-source";
    src = ../.;
    filter =
      path: _:
      let
        rel = lib.removePrefix "${toString ../.}/" (toString path);
        top = lib.head (lib.splitString "/" rel);
      in
      lib.elem top [
        "api"
        "cmd"
        "internal"
        "go.mod"
        "go.sum"
        "VERSION"
      ];
  };

  # The module cache rather than a vendor directory, for the reason nix/package.nix
  # states: `api` is a module of this repository reached by a filesystem replace, and
  # vendoring would fold it into the hash that pins third-party code.
  # The hash is that package's, since both builds download what one go.mod lists.
  proxyVendor = true;
  vendorHash = "sha256-YhwaORkqDclT1JBmo/V+jgICWEdhS6N9w5KQExJilEE=";

  subPackages = [ "cmd/groupd" ];

  # internal/receive is cgo and GStreamer, and no dependency of this binary reaches it.
  env.CGO_ENABLED = 0;

  meta = {
    description = "Group key, token and index service for the screen-sharing relay";
    mainProgram = "groupd";
    platforms = lib.platforms.linux;
  };
}
