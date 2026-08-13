{
  description = "screen-sharing: high-quality group screen sharing (MediaMTX relay + Go backend + Avalonia shell)";

  # A revision on both inputs and never a branch, so which package set a checkout builds
  # against is this file's property rather than the day someone last ran `nix flake update`.
  # A branch ref moves ffmpeg and GStreamer under the tree with no commit saying so, and
  # those two are what every codec verdict in internal/capabilities is measured against.
  #
  # The revision tracks nixos-unstable: a release channel trails GStreamer by a minor series,
  # and the plugin set is the thing under test.
  #
  # Updating is an edit, not a command.
  # docs/packaging.md ("Version pinning") states the procedure and what to re-measure.
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/2fcb964de67fcf60b43471c55d5d99e61a9ccb5a";
    flake-utils.url = "github:numtide/flake-utils/11707dc2f618dd54ca8739b309ec4fc024de578b";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    let
      # The relay build mediamtx.yml is written against, exported rather than kept inside
      # the dev shell.
      # A NixOS host serving as the relay needs that same build, and a second copy of the
      # pin is a second thing to bump.
      mediamtxOverlay = _: prev: {
        # mediamtx.yml turns on the Media over QUIC server, and MediaMTX rejects a config
        # carrying a key it does not know rather than ignoring it.
        # A relay without moqQUICAddress therefore does not start at all: it exits with
        # `ERR: json: unknown field "moq"`.
        # The key arrived in v1.20.0, and scripts/relay.ps1 downloads that same version for
        # Windows, where there is no dev shell to take a relay from.
        mediamtx = prev.mediamtx.overrideAttrs (
          finalAttrs: _: {
            version = "1.20.0";
            src = prev.fetchFromGitHub {
              owner = "bluenviron";
              repo = "mediamtx";
              tag = "v${finalAttrs.version}";
              hash = "sha256-bnbuIf3GdT+TCUHzAqvsS9wLPjDUGunpJoQBJFY4aTo=";
            };
            vendorHash = "sha256-uXwfIeE95g8isjR3ll0pcXnRtr/dbhp9B0HyH47WgWU=";
            # Replaced rather than reused: the package set's own postPatch names the
            # hls.js of the release it was written for and the rpicamera files this
            # version renamed, and each name it misses is a --replace-fail, so the phase
            # fails rather than skipping what it cannot find.
            #
            # It stands in for two go:generate steps that download what the build then
            # embeds, and the sandbox has no network for either.
            # hls.js is fetched as an input instead.
            # The Raspberry Pi camera binary has no such substitute, so the source that
            # embeds it is switched off: those files compile on 32- and 64-bit ARM Linux
            # alone, and with their build tags unsatisfiable every platform gets
            # source_other.go, which reports the camera as unsupported.
            postPatch =
              let
                # The hls.js version a release expects is in
                # internal/servers/hls/hlsjsdownloader/VERSION.
                hlsJs = prev.fetchurl {
                  url = "https://cdn.jsdelivr.net/npm/hls.js@v1.6.16/dist/hls.min.js";
                  hash = "sha256-RC9ZnDTxA8M1WzdaI73/VgWS1xF9CajIRyQuo94tQOA=";
                };
              in
              ''
                cp ${hlsJs} internal/servers/hls/hls.min.js
                echo "v${finalAttrs.version}" > internal/core/VERSION

                substituteInPlace internal/staticsources/rpicamera/{camera,camera_params,pipe,source,supports_hardware_h264}_arm_.go \
                  --replace-fail '(linux && arm) || (linux && arm64)' 'linux && !linux'
                substituteInPlace internal/staticsources/rpicamera/source_other.go \
                  --replace-fail '!linux || (!arm && !arm64)' 'linux || !linux'
                # These two are selected by filename rather than by a build tag, so
                # removing them is the only way to keep them out of an ARM build.
                # They hold nothing but the embed directive for the binary that is
                # now unreachable.
                rm internal/staticsources/rpicamera/camera_linux_arm.go \
                   internal/staticsources/rpicamera/camera_linux_arm64.go
              '';
          }
        );
      };
    in
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          # AMF and its encode core library ship as AMD's closed-source binaries.
          config.allowUnfreePredicate =
            pkg:
            builtins.elem (nixpkgs.lib.getName pkg) [
              "amf"
              "amdenc"
            ];
          overlays = [
            mediamtxOverlay
            (_: prev: {
              # Two plugins of gst-plugins-bad the cached build does not carry.
              #
              # The GStreamer publish engine's only Vulkan Video path is the vulkan
              # plugin's vulkanh264enc, with vulkanupload putting frames on the device it
              # encodes from.
              # nixpkgs configures gst-plugins-bad with "-Dvulkan=disabled" and carries no
              # Vulkan inputs, so the cached build has no plugin to load.
              # The loader and headers satisfy the meson feature, and shaderc provides the
              # glslc the plugin compiles its shaders with.
              #
              # The qsv plugin is both halves of the Intel path: qsvh264enc and its
              # siblings for the publish engine, qsvh264dec and its siblings for a receive
              # pipeline, which reaches an Intel GPU's 4:4:4 HEVC decoding that no va
              # element advertises.
              # Its meson option defaults to auto and nixpkgs passes no flag, so the plugin
              # is silently absent from the cached build.
              # It needs no new inputs: it vendors the oneVPL dispatcher and links the va
              # library gst-plugins-bad already builds, and only the Intel runtime it loads
              # at startup comes from outside (vplRuntime).
              gst_all_1 = prev.gst_all_1 // {
                gst-plugins-bad = prev.gst_all_1.gst-plugins-bad.overrideAttrs (old: {
                  mesonFlags =
                    builtins.filter (f: f != "-Dvulkan=disabled" && f != "-Dqsv=disabled") old.mesonFlags
                    ++ [
                      "-Dvulkan=enabled"
                      "-Dqsv=enabled"
                    ];
                  buildInputs = old.buildInputs ++ [
                    prev.vulkan-headers
                    prev.vulkan-loader
                  ];
                  nativeBuildInputs = old.nativeBuildInputs ++ [ prev.shaderc ];
                  # The plugin compiles each shader to SPIR-V and turns it into a C array
                  # with bin2array.py, whose "#!/usr/bin/env python3" the sandbox cannot
                  # resolve.
                  # patchShebangs covers build outputs, not a source script meson calls
                  # during the build, so this one is pointed at the interpreter by hand.
                  postPatch = (old.postPatch or "") + ''
                    patchShebangs ext/vulkan/shaders/bin2array.py
                  '';
                });
              };
            })
          ];
        };
        linuxDeps = with pkgs; [
          # cgo reads this to find the GStreamer development files gstDeps carries, which is
          # what internal/receive compiles against.
          pkg-config
          xrandr # X11 monitor enumeration (display pkg listX11)
        ];
        # Linux capture path: the kmsgrab pipeline, the unprivileged Wayland alternatives,
        # and the tools to inspect either.
        # kmsgrab needs CAP_SYS_ADMIN at runtime, which nix/screen-share.nix grants to a
        # dedicated ffmpeg-kmsgrab wrapper. Without that module, kmsgrab runs under sudo.
        linuxCaptureDeps = with pkgs; [
          wl-screenrec # wlroots screencopy, zero-copy DMA-BUF, hardware encode, no root
          wf-recorder # wlroots screencopy, ffmpeg-backed, no root
          wlr-randr # wlroots monitor enumeration (display pkg listWlrRandr)
          # VAAPI is split across two closures and vainfo is what checks them against each
          # other: the dispatch library comes from this shell, the driver it loads is the
          # host's, under /run/opengl-driver.
          # libva locates that driver through a version-stamped init symbol, probing from
          # its own VA-API version downwards, so a libva older than the one the host's Mesa
          # was built against opens no driver at all.
          # Both engines then lose every VAAPI element: the gst va plugin registers zero
          # features, and ffmpeg's vaapi device creation fails.
          # encoders.Detect greys every VAAPI row for it, which reads as a machine with no
          # hardware encode rather than as a version mismatch, so the nixpkgs pin is what
          # has to move.
          libva-utils # vainfo: confirm VAAPI encode entrypoints for hardware encode
          libdrm # modetest: enumerate CRTCs and planes to pick a kmsgrab device
          drm_info # DRM connector, plane, and format dump
        ];
        # GStreamer runs on both sides of the wire, in two processes.
        # Publish: pipewiresrc reads the xdg-desktop-portal ScreenCast node and the rest
        # encodes and ships over SRT or RTSP, in a gst-launch-1.0 child.
        # Watch: a receive pipeline decodes each stream through decodebin into an appsink
        # (internal/receive), linked into the backend itself, so this list is also what cgo
        # compiles that package against.
        #
        # Both sides find these plugins through GST_PLUGIN_SYSTEM_PATH_1_0, set in the
        # shellHook.
        # The shellHook replaces that variable rather than extending it, so anything either
        # side needs at runtime belongs here.
        gstDeps =
          with pkgs.gst_all_1;
          [
            gstreamer # gst-launch-1.0
            gst-plugins-base # videoconvert
            gst-plugins-good # pulsesrc (desktop audio), vpx (vp8enc/vp9enc), rtspsrc, progressreport, aacparse
            gst-plugins-bad # mpegtsmux, srtsink/srtsrc, h264parse/h265parse/av1parse, nvcodec, va (vah264enc and the other VAAPI encoders), qsv (Intel encode and decode), aom (av1enc), svtav1enc, opusenc
            gst-plugins-ugly # x264enc
            gst-rtsp-server # rtspclientsink
            gst-libav # avdec_h264/avdec_h265, the only decoders for H.264 4:4:4 and HEVC RExt (RGB). avenc_aac, the AAC audio encoder
            # rav1enc, rtpav1pay, whipclientsink, whepsrc.
            # rtspclientsink carries AV1 only through that payloader, and no other plugin
            # here has one.
            # whipclientsink is the GStreamer publish engine's WHIP sink, and whepsrc the
            # only reader of the relay's WHEP endpoint (docs/viewer-architecture.md).
            gst-plugins-rs
          ]
          ++ [
            pkgs.pipewire # pipewiresrc
            # nice elements: the ICE webrtcbin runs on, under both WebRTC legs,
            # whipclientsink publishing and whepsrc receiving.
            pkgs.libnice
          ];
        # AMD's hardware encoder, reached through ffmpeg's h264_amf, hevc_amf and av1_amf.
        # GStreamer has no Linux AMF path: its amfcodec plugin fails configuration with
        # "amf plugin supports only Windows", so the gstreamer publish backend stays on the
        # va elements and only the ffmpeg backend sees these encoders.
        # AMD ships the runtime for x86_64 alone.
        # The encoders take 4:2:0 alone, yuv420p and p010le, with RGB and 4:4:4 converted
        # on the way in.
        #
        # A Mesa RADV host needs AMF 1.4.37 or newer.
        # Releases up to 1.4.34 request the pre-standard VK_AMD_video_encode_queue and
        # VK_AMD_video_encode_h265 device extensions that only AMD's proprietary Vulkan
        # driver ever exposed.
        # RADV implements the ratified VK_KHR_video_encode_* set instead, so AMF resolves
        # those entry points to null and calls one anyway: ffmpeg dies with SIGSEGV inside
        # AMFDeviceVulkanImpl::CreateDeviceAndFindQueues.
        # 1.4.37 drops the requirement and drives the encoder through libamdenc64.
        #
        # LD_LIBRARY_PATH is how the shell's processes find the runtime, and it reaches an
        # unprivileged ffmpeg alone.
        # A capability-bearing binary runs in glibc's secure-execution mode, where that
        # variable is ignored, so the kmsgrab wrapper takes the runtime on libavutil's
        # RUNPATH instead (nix/screen-share.nix, the amf option).
        amfRuntime = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isx86_64 [ pkgs.amf ];
        # Intel's oneVPL runtime, the implementation behind every QSV encoder and decoder.
        # Both engines reach it through a dispatcher that loads the runtime by filename at
        # startup: ffmpeg links libvpl (--enable-libvpl) and the qsv plugin vendors the same
        # dispatcher.
        # Neither finds a store path on its own, which is what ONEVPL_SEARCH_PATH answers.
        # Intel ships the runtime for x86_64 alone.
        # On a machine with no Intel GPU it loads and reports no hardware implementation,
        # which is the encoder probe's answer rather than a table fact.
        vplRuntime = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isx86_64 [ pkgs.vpl-gpu-rt ];
        # The Avalonia shell's toolchain (avalonia/README.md).
        # The SDK builds, runs and tests that module, and Directory.Build.props pins net10.0
        # because Avalonia 12 targets it, so the 10 in the attribute name is the version the
        # projects will not build without.
        #
        # protobuf and grpc are here for a reason specific to this package set.
        # Grpc.Tools compiles api/proto during the build with prebuilt protoc and
        # grpc_csharp_plugin binaries from its NuGet package, and those are linked against
        # /lib64/ld-linux-x86-64.so.2, an interpreter NixOS does not have.
        # The build then dies with "cannot execute: required file not found" before a single
        # .cs file is generated, so the shellHook points Grpc.Tools at this pair instead.
        # Nothing else in the repo consumes them: the Go side generates through buf, which
        # carries its own compiler.
        dotnetDeps = with pkgs; [
          dotnet-sdk_10
          protobuf
          grpc
        ];
        # What the Avalonia app resolves by soname at run time, and therefore what has to be
        # on the loader path rather than merely in the shell closure.
        # Three sources: the X11 backend dlopens libX11 and its extensions, the Wayland
        # backend does the same with the compositor libraries, the keymap library and the
        # buffer allocator behind its EGL surfaces, and Skia, the renderer behind every pixel
        # Avalonia draws, arrives as a prebuilt libSkiaSharp.so from NuGet that expects
        # fontconfig, freetype and libstdc++ to be findable the same way.
        # None of the three passes through a derivation, so nix patches none of them.
        #
        # Both windowing backends are carried because which one runs is the session's answer
        # rather than the build's: the app asks for Wayland and takes X11 where there is no
        # compositor to ask (avalonia/ScreenShare.App/Program.cs).
        #
        # Unlike the AMF directory, this list does shadow libraries the rest of the shell
        # already uses.
        # It comes from the same package set, so it resolves to the store paths they link
        # against and the shadowing is nominal for as long as that holds.
        avaloniaRuntimeDeps = with pkgs; [
          fontconfig
          freetype
          libglvnd # libGL and libEGL: both backends' GPU render path
          stdenv.cc.cc.lib # libstdc++, for libSkiaSharp
          # The Wayland backend's own set: the compositor libraries it draws through, the
          # keymap library every Wayland client turns key codes with, and the buffer
          # allocator its EGL surfaces come from.
          # glib carries the event loop that backend runs on.
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
      in
      {
        # The installable app: both binaries, each wrapped so it finds the programs and
        # libraries it resolves at run time (nix/package.nix).
        #
        #   nix run github:bjoern621/screen-sharing
        #   environment.systemPackages = [ screen-sharing.packages.${system}.default ];
        packages.screen-sharing = pkgs.callPackage ./nix/package.nix { };
        packages.default = self.packages.${system}.screen-sharing;

        # The service that runs beside the relay rather than on a desktop (nix/groupd.nix).
        # A NixOS host installs it through the overlay below.
        packages.groupd = pkgs.callPackage ./nix/groupd.nix { };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.screen-sharing}/bin/screenshare-avalonia";
        };

        devShells.default = pkgs.mkShell {
          packages =
            with pkgs;
            [
              go
              # The full build for x11grab/kmsgrab, ffplay, and the software encoder
              # libraries: libvpx, libaom, SVT-AV1 and rav1e are optional build inputs, and
              # encoders.Detect greys whichever the build lacks.
              ffmpeg-full
              mpv # single-stream viewer, selected by SCREENSHARE_VIEWER below
              mediamtx # the relay, run natively: `mediamtx mediamtx.yml`
              go-task # runs Taskfile.yml
              nil
              nixfmt
            ]
            ++ dotnetDeps
            ++ pkgs.lib.optionals pkgs.stdenv.isLinux (
              linuxDeps ++ linuxCaptureDeps ++ gstDeps ++ amfRuntime ++ vplRuntime ++ avaloniaRuntimeDeps
            );

          shellHook = ''
            echo "screen-sharing dev shell - run 'task' for available commands"
            echo "kmsgrab needs CAP_SYS_ADMIN: enable nix/screen-share.nix for the"
            echo "ffmpeg-kmsgrab wrapper, or run plain ffmpeg kmsgrab under sudo."

            # watch.Select reads this; mpv is the viewer for this shell. Unset it
            # to fall back to the in-code default, ffplay.
            export SCREENSHARE_VIEWER=mpv

            # Grpc.Tools resolves both of these itself when they are empty, to the
            # prebuilt binaries in its NuGet package that this platform cannot run.
            # MSBuild reads the environment as global properties and Grpc.Tools sets
            # both under a "is it still empty" condition, so naming them here is the
            # whole override - no property in a .csproj, and nothing a Windows
            # checkout of the same tree picks up.
            export Protobuf_ProtocFullPath="${pkgs.protobuf}/bin/protoc"
            export gRPC_PluginFullPath="${pkgs.grpc}/bin/grpc_csharp_plugin"

            # The first `dotnet` of a session otherwise prints a telemetry notice
            # into whatever output the task runner is showing.
            export DOTNET_CLI_TELEMETRY_OPTOUT=1
          ''
          + pkgs.lib.optionalString pkgs.stdenv.isLinux ''
            export GST_PLUGIN_SYSTEM_PATH_1_0="${
              pkgs.lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstDeps
            }"
          ''
          + pkgs.lib.optionalString (pkgs.stdenv.isLinux && vplRuntime != [ ]) ''
            # The oneVPL dispatcher searches the distro library paths for the runtime,
            # which hold nothing on NixOS, and both engines then report no hardware
            # implementation on a machine that has one. This is the search path it reads
            # in addition to its own; it names one directory holding one library, so it
            # cannot shadow anything else the shell loads.
            export ONEVPL_SEARCH_PATH="${pkgs.lib.makeLibraryPath vplRuntime}"
          ''
          + pkgs.lib.optionalString (pkgs.stdenv.isLinux && amfRuntime != [ ]) ''
            # ffmpeg dlopens libamfrt64.so.1 by soname, so the AMF runtime has to
            # be on the loader path and not merely in the shell closure. The
            # directory holds nothing but libamfrt64, so it shadows no system
            # library for the processes that inherit this.
            export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath amfRuntime}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
          ''
          + pkgs.lib.optionalString pkgs.stdenv.isLinux ''
            # The X11 backend and libSkiaSharp dlopen their way to these, so a shell
            # that only carried them as build inputs would still fail at the first
            # window: `task avalonia` dies in Avalonia's platform init, before any
            # code in the app runs.
            export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath avaloniaRuntimeDeps}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
          ''
          + pkgs.lib.optionalString pkgs.stdenv.isLinux ''
            # Hyprland tiles every toplevel it is not told to float, and no window can
            # ask for anything else: Wayland has no client request for it. The shell is
            # a desktop app and not a pane, so the rule is stated here, for the
            # compositor session the dev shell is entered from.
            #
            # It matches an empty class because Avalonia.Wayland carries no set_app_id
            # request, which leaves the window with no app_id for a rule to name it by.
            # The title is what remains, and ShellWindow binds it to the destination
            # label (Features/Shell/Model/Destinations.cs), with the session appended
            # once there is one.
            #
            # A Lua config refuses `hyprctl keyword` outright ("keyword can't work with
            # non-legacy parsers"), so the rule goes in through hl.window_rule, and the
            # keyword is the same rule for a legacy config. A compositor that takes
            # neither says so, rather than leaving a tiled window unexplained.
            #
            # The Lua global is the idempotency guard, and it is an exact one: a config
            # reload drops the rule and the global together, so the flag is set only
            # while the rule it stands for is installed.
            #
            # LD_LIBRARY_PATH is cleared for the call. hyprctl links a newer libstdc++
            # than the one this shell puts ahead of its RUNPATH for Skia, and with the
            # shell's copy in front it exits before it reaches the socket.
            if [ -n "''${HYPRLAND_INSTANCE_SIGNATURE:-}" ] && command -v hyprctl > /dev/null; then
              hyprFloat=$(env -u LD_LIBRARY_PATH hyprctl eval '
                if not _G.__screenshare_float then
                  _G.__screenshare_float = true
                  hl.window_rule({
                    match = { class = "^$", initial_title = "^(Setup|Broadcast|Viewer)( .*)?$" },
                    float = true,
                  })
                end' 2>&1)
              if [ "$hyprFloat" != ok ]; then
                hyprFloat=$(env -u LD_LIBRARY_PATH hyprctl keyword windowrule \
                  'float, class:^$, initialTitle:^(Setup|Broadcast|Viewer)( .*)?$' 2>&1)
              fi
              [ "$hyprFloat" = ok ] || echo "hyprland: the shell window opens tiled ($hyprFloat)"
              unset hyprFloat
            fi
          '';
        };
      }
    )
    // {
      # Imported by a host config to get the privileged kmsgrab wrapper:
      #   imports = [ screen-sharing.nixosModules.screenShare ];
      #   programs.screenShare = { enable = true; user = "bjoern"; };
      nixosModules.screenShare = import ./nix/screen-share.nix;
      nixosModules.default = self.nixosModules.screenShare;

      # The MediaMTX build mediamtx.yml is written against, for a host serving as the relay:
      #   nixpkgs.overlays = [ screen-sharing.overlays.mediamtx ];
      # The dev shell applies the same overlay, so `task relay` and a deployed relay are one
      # version.
      overlays.mediamtx = mediamtxOverlay;

      # The group service, for that same host: it runs beside the relay because that is where
      # the signing key lives and where the relay fetches it from.
      #   nixpkgs.overlays = [ screen-sharing.overlays.groupd ];
      #   systemd.services.groupd.serviceConfig.ExecStart = lib.getExe pkgs.screenshare-groupd;
      overlays.groupd = final: _: {
        screenshare-groupd = final.callPackage ./nix/groupd.nix { };
      };

      # A relay host wants both halves, so the default is the pair: a host that took the relay
      # alone would serve tokens nobody signs.
      overlays.default = nixpkgs.lib.composeManyExtensions [
        self.overlays.mediamtx
        self.overlays.groupd
      ];
    };
}
