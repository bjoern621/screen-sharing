# The app as one package: the Go backend, the Avalonia shell, and a desktop entry.
#
# The two binaries are one install because the shell starts the backend itself when
# nothing answers on the control endpoint, looking for that binary beside itself and
# then on PATH (avalonia/ScreenShare.App/Backend/BackendProcess.cs). Both are wrapped
# rather than left to the environment: ffmpeg, the GStreamer launcher and the plugin
# directory live at store paths no login PATH carries.
#
# ffmpeg is a closure dependency rather than a bundled copy, which is the model
# docs/packaging.md picks for every channel with a package manager behind it.
{
  lib,
  symlinkJoin,
  makeWrapper,
  buildGoModule,
  buildDotnetModule,
  dotnet-sdk_10,
  pkg-config,
  ffmpeg-full,
  gst_all_1,
  libnice,
  pipewire,
  protobuf,
  grpc,
  # Avalonia resolves these by soname at run time rather than linking them, so they have
  # to reach LD_LIBRARY_PATH. The list is the dev shell's, and for the same reason: Skia
  # arrives prebuilt from NuGet and both windowing backends dlopen their way to the
  # session's libraries (flake.nix, avaloniaRuntimeDeps).
  fontconfig,
  freetype,
  libglvnd,
  stdenv,
  glib,
  libdrm,
  libgbm,
  libxkbcommon,
  wayland,
  libice,
  libsm,
  libx11,
  libxcursor,
  libxext,
  libxi,
  libxrandr,
  version ? lib.fileContents ../VERSION,
}:

let
  # The inputs the two builds read, and nothing else. The working tree also holds the
  # .NET intermediate and output directories and the node_modules of the deleted Wails
  # frontend, and a source carrying those would be gigabytes and would change on every
  # local build.
  src = lib.cleanSourceWith {
    name = "screen-sharing-source";
    src = ../.;
    filter =
      path: type:
      let
        rel = lib.removePrefix "${toString ../.}/" (toString path);
        top = lib.head (lib.splitString "/" rel);
      in
      lib.elem top [
        "api"
        "cmd"
        "internal"
        "avalonia"
        "go.mod"
        "go.sum"
        "VERSION"
      ]
      && !(
        type == "directory"
        && lib.elem (baseNameOf path) [
          "obj"
          "bin"
        ]
      );
  };

  # Both engines' GStreamer, in one list. A publish spawns gst-launch-1.0 and a receive
  # pipeline runs in the backend itself, so one plugin set serves both; the reason each
  # package is here is stated beside the dev shell's copy of the list in flake.nix.
  gstPlugins = with gst_all_1; [
    gstreamer
    gst-plugins-base
    gst-plugins-good
    gst-plugins-bad
    gst-plugins-ugly
    gst-rtsp-server
    gst-libav
    gst-plugins-rs
    pipewire
    libnice
  ];

  gstPluginPath = lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstPlugins;

  avaloniaRuntimeDeps = [
    fontconfig
    freetype
    libglvnd
    stdenv.cc.cc.lib
    glib
    libdrm
    libgbm
    libxkbcommon
    wayland
    libice
    libsm
    libx11
    libxcursor
    libxext
    libxi
    libxrandr
  ];

  backend = buildGoModule {
    pname = "screenshare-backend";
    inherit version src;

    # The module cache rather than a vendor directory, because `api` is a module of this
    # repository reached by a filesystem `replace` (go.mod). Vendoring copies it into the
    # fixed-output derivation the hash below pins, so every edit under api/ invalidates a
    # hash that names third-party dependencies, and a store already holding that path
    # builds the old generated code instead: the compiler then reports a symbol the
    # working tree defines as undefined. A proxy fetch downloads what go.sum lists and
    # nothing else, so the pin covers third-party code alone and the local module is read
    # from src at build time.
    proxyVendor = true;
    vendorHash = "sha256-YhwaORkqDclT1JBmo/V+jgICWEdhS6N9w5KQExJilEE=";

    subPackages = [ "cmd/backend" ];

    # internal/receive is cgo throughout: it builds GStreamer pipelines in-process and
    # imports the decoded frames through EGL, and pkg-config is how those headers are
    # found (the module names are in share_linux.go).
    nativeBuildInputs = [
      pkg-config
      makeWrapper
    ];
    buildInputs = [
      gst_all_1.gstreamer
      gst_all_1.gst-plugins-base
      libglvnd
    ];

    # The package name is the directory's, and the binary the rest of the repository
    # names is not. The rename comes before the wrapper so the wrapper is written
    # against the final name.
    postInstall = ''
      mv $out/bin/backend $out/bin/screenshare-backend

      wrapProgram $out/bin/screenshare-backend \
        --prefix PATH : ${
          lib.makeBinPath [
            ffmpeg-full
            gst_all_1.gstreamer
          ]
        } \
        --set-default GST_PLUGIN_SYSTEM_PATH_1_0 "${gstPluginPath}"
    '';

    meta.mainProgram = "screenshare-backend";
  };

  shell = buildDotnetModule {
    pname = "screenshare-avalonia";
    inherit version src;

    projectFile = "avalonia/ScreenShare.App/ScreenShare.App.csproj";
    nugetDeps = ./deps.json;
    dotnet-sdk = dotnet-sdk_10;
    executables = [ "screenshare-avalonia" ];

    runtimeDeps = avaloniaRuntimeDeps;

    # Grpc.Tools compiles api/proto during this build with prebuilt binaries from its
    # NuGet package, which are linked against an interpreter this platform does not have.
    # Naming the pair in the environment is the whole override, the same way the dev
    # shell does it (flake.nix).
    nativeBuildInputs = [
      protobuf
      grpc
    ];
    env = {
      Protobuf_ProtocFullPath = "${protobuf}/bin/protoc";
      gRPC_PluginFullPath = "${grpc}/bin/grpc_csharp_plugin";
    };

    # The shell starts the backend when nothing answers on the control endpoint, looking
    # beside its own assembly first and then on PATH. Beside finds nothing here, because
    # the assemblies live under lib and the binaries under bin, so PATH is the whole of
    # how the two halves meet in this package.
    makeWrapperArgs = [
      "--prefix"
      "PATH"
      ":"
      "${backend}/bin"
    ];

    meta.mainProgram = "screenshare-avalonia";
  };
in
symlinkJoin {
  name = "screen-sharing-${version}";
  inherit version;

  paths = [
    backend
    shell
  ];

  # Both binaries land in one bin directory, which is the layout the shell's own lookup
  # expects, and the desktop entry is what a menu launch goes through. The entry is the
  # one every Linux channel installs, so a menu shows the same app whichever built it.
  postBuild = ''
    install -Dm444 ${../packaging/linux/screen-sharing.desktop} \
      $out/share/applications/screen-sharing.desktop
    install -Dm444 ${../build/appicon.png} \
      $out/share/icons/hicolor/1024x1024/apps/screen-sharing.png

    install -Dm444 ${../LICENSE} $out/share/licenses/screen-sharing/LICENSE
    install -Dm444 ${../THIRD-PARTY-NOTICES.md} \
      $out/share/doc/screen-sharing/THIRD-PARTY-NOTICES.md
  '';

  passthru = {
    inherit backend shell;
    # `nix run .#screen-sharing.fetch-deps -- nix/deps.json` rewrites the NuGet lock,
    # which a changed PackageReference needs before this package builds again.
    inherit (shell) fetch-deps;
  };

  meta = {
    description = "Self-hosted, high-quality group screen sharing";
    homepage = "https://github.com/bjoern621/screen-sharing";
    mainProgram = "screenshare-avalonia";
    platforms = lib.platforms.linux;
    # The project's own code. ffmpeg and GStreamer reach the wrapper from the closure
    # under their own terms, which is what a dependency rather than a bundled copy means.
    license = lib.licenses.asl20;
  };
}
