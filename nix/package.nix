# The app as one package: the Go backend, the Avalonia shell, and a desktop entry.
#
# One install for two binaries,
# the shell starting the backend when nothing answers on the control endpoint,
# beside itself and then on PATH (avalonia/ScreenShare.App/Backend/BackendProcess.cs).
# Both are wrapped rather than left to the environment:
# ffmpeg, the GStreamer launcher and the plugin directory live at store paths no login PATH carries.
#
# ffmpeg is a closure dependency and not a bundled copy,
# the model docs/packaging.md picks for every channel with a package manager behind it.
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
  imagemagick,
  glib-networking,
  libnice,
  pipewire,
  protobuf,
  grpc,
  # The backend spawns these to enumerate monitors, one per session type
  # (backend/internal/display, listX11 and listWlrRandr).
  # Without them the enumeration answers a single placeholder with no geometry,
  # so the setup screen offers one nameless screen and refuses every monitor but the first.
  xrandr,
  wlr-randr,
  # The hardware encoder runtimes ffmpeg loads by name rather than links,
  # each reached through an environment variable and neither able to find a store path on its own.
  # flake.nix states what each one is and why the dev shell sets the same two.
  amf,
  vpl-gpu-rt,
  # Avalonia dlopens these by soname at run time rather than linking them,
  # so they have to reach LD_LIBRARY_PATH or the shell fails to open a window.
  # Skia arrives prebuilt from NuGet and both windowing backends find the session's libraries
  # the same way, so the list matches the dev shell's (flake.nix, avaloniaRuntimeDeps).
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
  # The inputs the two builds read, and nothing else.
  # A working tree also holds the .NET intermediate and output directories,
  # so a source carrying those would be gigabytes and would change on every local build.
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
        "backend"
        "avalonia"
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

  # One plugin set for both engines:
  # a publish spawns gst-launch-1.0 and a receive pipeline runs in the backend itself,
  # and every element either builds comes from here.
  # What each package supplies is stated beside the dev shell's copy of the list in flake.nix.
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

  # The icon sizes hicolor's index declares, which are the ones a lookup walks.
  # Every Linux channel installs the same set, so a menu draws one icon whichever built it.
  iconSizes = [
    48
    64
    128
    256
    512
  ];

  # The monitor enumerators, which are Linux session tools and have no counterpart elsewhere.
  # pipewire carries pw-dump, which the audio device list subscribes to for the daemon's
  # add and remove events (internal/audiodev).
  # A closure without it enumerates nothing and every audio kind keeps its own default,
  # which reads as a machine that plays no sound rather than as a missing tool.
  displayTools = lib.optionals stdenv.hostPlatform.isLinux [
    xrandr
    wlr-randr
    pipewire
  ];

  # AMD and Intel ship these runtimes for x86_64 alone,
  # so the wrapper names neither anywhere else and the encoders they back are absent there.
  amfRuntime = lib.optionals stdenv.hostPlatform.isx86_64 [ amf ];
  vplRuntime = lib.optionals stdenv.hostPlatform.isx86_64 [ vpl-gpu-rt ];

  # A packaged build hands these over the way the dev shell does,
  # or an encoder working inside `nix develop` is greyed out under `nix run` on the same machine:
  # ffmpeg dlopens libamfrt64.so.1 by soname,
  # the oneVPL dispatcher loads its runtime by filename,
  # and neither carries a store path (docs/packaging.md, flake.nix).
  hardwareRuntimeArgs = lib.concatStringsSep " " (
    lib.optional (
      vplRuntime != [ ]
    ) ''--set-default ONEVPL_SEARCH_PATH "${lib.makeLibraryPath vplRuntime}"''
    ++ lib.optional (
      amfRuntime != [ ]
    ) ''--prefix LD_LIBRARY_PATH : "${lib.makeLibraryPath amfRuntime}"''
  );

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

    # The module cache rather than a vendor directory,
    # `api` being a module of this repository reached by a filesystem `replace` (backend/go.mod).
    # Vendoring copies it into the fixed-output derivation the hash below pins,
    # so an edit under api/ invalidates a hash that names third-party dependencies,
    # and a store already holding that path builds the stale generated code:
    # the compiler then reports a symbol the working tree defines as undefined.
    # A proxy fetch downloads what backend/go.sum lists and nothing else,
    # so the pin covers third-party code alone and the local module is read from src at build time.
    proxyVendor = true;
    vendorHash = "sha256-YhwaORkqDclT1JBmo/V+jgICWEdhS6N9w5KQExJilEE=";

    # The Go module is backend/, not the repository root, where api/ and avalonia/ sit beside it.
    modRoot = "backend";
    subPackages = [ "cmd/backend" ];

    # backend/internal/receive is cgo throughout:
    # it builds GStreamer pipelines in-process and imports the decoded frames through EGL,
    # and pkg-config is what finds those headers (the module names are in share_linux.go).
    # libx11 is backend/internal/pointer's: the cursor position is read through Xlib,
    # and a build without it fails at the pointer package rather than anywhere GStreamer is named.
    nativeBuildInputs = [
      pkg-config
      makeWrapper
    ];
    buildInputs = [
      gst_all_1.gstreamer
      gst_all_1.gst-plugins-base
      libglvnd
      libx11
    ];

    # buildGoModule names the binary after its directory,
    # and the rest of the repository names it screenshare-backend.
    # The rename comes before the wrapper, so the wrapper is written against the final name.
    #
    # GLib carries no TLS of its own and takes it from a GIO module,
    # so without glib-networking every rtsps:// and https:// leg fails at the connect.
    # rtspclientsink reports "Failed to connect. (Generic error)" there,
    # which names neither TLS nor the missing module.
    # A prefix rather than a set, the session's own value carrying the desktop's modules.
    postInstall = ''
      mv $out/bin/backend $out/bin/screenshare-backend

      wrapProgram $out/bin/screenshare-backend \
        --prefix PATH : ${
          lib.makeBinPath (
            [
              ffmpeg-full
              gst_all_1.gstreamer
            ]
            ++ displayTools
          )
        } \
        --set-default GST_PLUGIN_SYSTEM_PATH_1_0 "${gstPluginPath}" \
        --prefix GIO_EXTRA_MODULES : "${glib-networking}/lib/gio/modules" \
        ${hardwareRuntimeArgs}
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

    # Grpc.Tools compiles api/proto during this build with prebuilt binaries from its NuGet package,
    # linked against an interpreter this platform does not have.
    # Naming the two paths in the environment is the whole override, as the dev shell does (flake.nix).
    nativeBuildInputs = [
      protobuf
      grpc
    ];
    env = {
      Protobuf_ProtocFullPath = "${protobuf}/bin/protoc";
      gRPC_PluginFullPath = "${grpc}/bin/grpc_csharp_plugin";
    };

    # The shell looks for the backend beside its own assembly first and then on PATH.
    # Beside finds nothing here, the assemblies living under lib and the binaries under bin,
    # so PATH is the whole of how the two halves meet in this package.
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

  nativeBuildInputs = [ imagemagick ];

  # The join puts both binaries in one bin directory, the layout the shell's lookup expects.
  # The desktop entry is what a menu launch goes through, and every Linux channel installs it,
  # so a menu shows the same app whichever built it.
  postBuild = ''
    install -Dm444 ${../packaging/linux/screen-sharing.desktop} \
      $out/share/applications/screen-sharing.desktop

    # The master is 1024px and hicolor's index declares 48 through 512.
    # A size the index does not name is a directory no lookup walks,
    # so an icon installed at 1024 alone is one a launcher answers with its placeholder.
    for size in ${lib.concatStringsSep " " (map toString iconSizes)}; do
      dir=$out/share/icons/hicolor/''${size}x''${size}/apps
      mkdir -p $dir
      magick ${../build/appicon.png} -resize ''${size}x''${size} $dir/screen-sharing.png
      chmod 444 $dir/screen-sharing.png
    done

    install -Dm444 ${../LICENSE} $out/share/licenses/screen-sharing/LICENSE
    install -Dm444 ${../THIRD-PARTY-NOTICES.md} \
      $out/share/doc/screen-sharing/THIRD-PARTY-NOTICES.md
  '';

  passthru = {
    inherit backend shell;
    # A changed PackageReference needs the NuGet lock rewritten before this package builds again:
    # `nix run .#screen-sharing.fetch-deps -- nix/deps.json`.
    inherit (shell) fetch-deps;
  };

  meta = {
    description = "Self-hosted, high-quality group screen sharing";
    homepage = "https://github.com/bjoern621/screen-sharing";
    mainProgram = "screenshare-avalonia";
    platforms = lib.platforms.linux;
    # The project's own code alone.
    # ffmpeg and GStreamer reach the wrapper from the closure under their own terms,
    # which is what a dependency rather than a bundled copy means.
    license = lib.licenses.asl20;
  };
}
