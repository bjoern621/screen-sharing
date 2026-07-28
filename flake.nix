{
  description = "screen-sharing: high-quality group screen sharing (MediaMTX relay + Wails app)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
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
        # WebKitGTK gets its WebRTC bindings from ENABLE_WEB_RTC, which rides on
        # ENABLE_EXPERIMENTAL_FEATURES. The nixpkgs default leaves that off, so
        # the stock webview has no RTCPeerConnection at all and every WHEP tile
        # in the grid fails on the missing constructor. cache.nixos.org carries
        # the experimental build, so this costs a download and not a WebKit
        # compile.
        #
        # It is an overlay rather than a list entry because the wails package
        # pulls webkitgtk in too, and whichever copy lands first on
        # PKG_CONFIG_PATH is the one the app links against.
        pkgs = import nixpkgs {
          inherit system;
          # AMF and its encode core library are AMD's closed-source binaries.
          config.allowUnfreePredicate =
            pkg:
            builtins.elem (nixpkgs.lib.getName pkg) [
              "amf"
              "amdenc"
            ];
          overlays = [
            (final: prev: {
              webkitgtk_4_1 = prev.webkitgtk_4_1.override { enableExperimental = true; };

              # The GStreamer publish engine's only Vulkan Video path is the vulkan
              # plugin's vulkanh264enc, with vulkanupload putting frames on the device
              # it encodes from. nixpkgs configures gst-plugins-bad with
              # "-Dvulkan=disabled" and carries no Vulkan inputs at all ("we haven't
              # figured out yet which of the vulkan nixpkgs it needs"), so the cached
              # build has no plugin to load. Enabling it costs a source build of that
              # one package: the loader and headers satisfy the meson feature, and
              # shaderc provides the glslc the plugin compiles its shaders with.
              gst_all_1 = prev.gst_all_1 // {
                gst-plugins-bad = prev.gst_all_1.gst-plugins-bad.overrideAttrs (old: {
                  mesonFlags = builtins.filter (f: f != "-Dvulkan=disabled") old.mesonFlags ++ [
                    "-Dvulkan=enabled"
                  ];
                  buildInputs = old.buildInputs ++ [
                    prev.vulkan-headers
                    prev.vulkan-loader
                  ];
                  nativeBuildInputs = old.nativeBuildInputs ++ [ prev.shaderc ];
                  # The plugin compiles each shader to SPIR-V and turns it into a C
                  # array with bin2array.py, whose "#!/usr/bin/env python3" line the
                  # sandbox cannot resolve. patchShebangs runs over build outputs, not
                  # over a source script meson calls during the build, so this one is
                  # pointed at the interpreter by hand.
                  postPatch = (old.postPatch or "") + ''
                    patchShebangs ext/vulkan/shaders/bin2array.py
                  '';
                });
              };

              # AMF talks to the hardware encoder through Vulkan. Releases up to
              # 1.4.34 request the pre-standard VK_AMD_video_encode_queue and
              # VK_AMD_video_encode_h265 device extensions, which only AMD's
              # proprietary Vulkan driver ever exposed. AMD deprecated that driver
              # in favour of Mesa RADV, and RADV implements the ratified
              # VK_KHR_video_encode_* set instead, so AMF resolves the AMD entry
              # points to null and calls one anyway: ffmpeg dies with SIGSEGV
              # inside AMFDeviceVulkanImpl::CreateDeviceAndFindQueues.
              #
              # 1.4.37 drops the VK_AMD_video_encode_* requirement and drives the
              # encoder through libamdenc64, so the *_amf encoders work against
              # RADV. It ships in the amdgpu 6.4.4 repository, one release train
              # ahead of the nixpkgs pin, and needs the matching amdenc from the
              # same build (2203192).
              amdenc = prev.amdenc.overrideAttrs (
                finalAttrs: _: {
                  version = "25.10-2203192";
                  src = prev.fetchurl {
                    url = "https://repo.radeon.com/amdgpu/6.4.4/ubuntu/pool/proprietary/liba/libamdenc-amdgpu-pro/libamdenc-amdgpu-pro_${finalAttrs.version}.24.04_amd64.deb";
                    hash = "sha256-jEvHZxTzN8TzZJuouYaOGw9xaRINA/zEg+56s/13ruw=";
                  };
                }
              );
              amf = (prev.amf.override { inherit (final) amdenc; }).overrideAttrs (
                finalAttrs: _: {
                  version = "1.4.37-2203192";
                  src = prev.fetchurl {
                    url = "https://repo.radeon.com/amdgpu/6.4.4/ubuntu/pool/proprietary/a/amf-amdgpu-pro/amf-amdgpu-pro_${finalAttrs.version}.24.04_amd64.deb";
                    hash = "sha256-pklpKaWLrcClRRaY9jJhFZLbyFXPUY9H5UpmARrgFPU=";
                  };
                }
              );
            })
          ];
        };
        linuxDeps = with pkgs; [
          gtk3
          webkitgtk_4_1 # Wails v2 Linux backend (build with tag webkit2_41)
          pkg-config
          xrandr # X11 monitor enumeration (display pkg listX11)
        ];
        # Linux capture path: the kmsgrab pipeline plus the unprivileged Wayland
        # alternatives, and the tools to inspect either. kmsgrab still needs
        # CAP_SYS_ADMIN at runtime; the nix/screen-share.nix module grants that to a
        # dedicated ffmpeg-kmsgrab wrapper. Without the module, run kmsgrab under sudo.
        linuxCaptureDeps = with pkgs; [
          wl-screenrec # wlroots screencopy, zero-copy DMA-BUF, hardware encode, no root
          wf-recorder # wlroots screencopy, ffmpeg-backed, no root
          wlr-randr # wlroots monitor enumeration (display pkg listWlrRandr)
          libva-utils # vainfo: confirm VAAPI encode entrypoints for hardware encode
          libdrm # modetest: enumerate CRTCs and planes to pick a kmsgrab device
          drm_info # DRM connector, plane, and format dump
        ];
        # GStreamer runs on both sides of the wire. Publish: pipewiresrc reads
        # the xdg-desktop-portal ScreenCast node, the rest encodes and ships
        # over SRT or RTSP. Watch: the native grid binary decodes each stream
        # through decodebin into a gtk4paintablesink; it inherits this plugin
        # path from the app that spawns it. gst-launch-1.0 finds these plugins
        # through GST_PLUGIN_SYSTEM_PATH_1_0 (set in the shellHook).
        #
        # The shellHook replaces that variable rather than extending it, so this
        # list is also the only plugin path WebKitGTK sees. Anything the webview
        # needs at runtime belongs here too, libnice included: where WebKit has
        # the WebRTC bindings it implements RTCPeerConnection on webrtcbin, which
        # refuses to start without the nice elements. The nixpkgs webkitgtk is
        # built without those bindings (see docs/viewer-architecture.md), so
        # libnice only matters against a webkitgtk built with experimental
        # features on.
        gstDeps =
          with pkgs.gst_all_1;
          [
            gstreamer # gst-launch-1.0
            gst-plugins-base # videoconvert
            gst-plugins-good # pulsesrc (desktop audio), vpx (vp8enc/vp9enc), rtspsrc, progressreport
            gst-plugins-bad # mpegtsmux, srtsink/srtsrc, h264parse/h265parse/av1parse, nvcodec, va (vah264enc and the other VAAPI encoders), aom (av1enc), svtav1enc, opusenc
            gst-plugins-ugly # x264enc
            gst-rtsp-server # rtspclientsink
            gst-libav # avdec_h264/avdec_h265: the only decoders for H.264 4:4:4 and HEVC RExt (RGB)
            # gtk4paintablesink (native grid video sink), rav1enc, rtpav1pay and
            # whipclientsink: rtspclientsink needs the payloader to carry AV1, and no
            # other plugin here has one, and whipclientsink is the GStreamer publish
            # engine's WHIP sink.
            gst-plugins-rs
          ]
          ++ [
            pkgs.pipewire # pipewiresrc gst plugin
            pkgs.libnice # nice elements: ICE for webrtcbin, i.e. WHEP in the webview
          ];
        # AMD's hardware encoder, reachable through ffmpeg's h264_amf, hevc_amf
        # and av1_amf. GStreamer has no Linux AMF path at all: its amfcodec
        # plugin fails configuration with "amf plugin supports only Windows", so
        # the gstreamer publish backend stays on the va elements and only the
        # ffmpeg backend sees these encoders. AMD ships the runtime for x86_64
        # alone. The encoders remain 4:2:0: yuv420p and p010le, with RGB and
        # 4:4:4 input converted on the way in.
        amfRuntime = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isx86_64 [ pkgs.amf ];
      in
      {
        devShells.default = pkgs.mkShell {
          packages =
            with pkgs;
            [
              go
              nodejs_22
              wails
              # ffmpeg-full for x11grab/kmsgrab, ffplay, and the software encoder
              # libraries: libvpx, libaom, SVT-AV1 and rav1e are all optional build
              # inputs, and encoders.Detect greys whichever the build lacks.
              ffmpeg-full
              mpv # single-stream viewer, selected by SCREENSHARE_VIEWER below
              mediamtx # the relay, run natively: `mediamtx mediamtx.yml`
              go-task # task runner, see Taskfile.yml
              # Nix
              nil
              nixfmt
            ]
            ++ pkgs.lib.optionals pkgs.stdenv.isLinux (linuxDeps ++ linuxCaptureDeps ++ gstDeps ++ amfRuntime);

          shellHook = ''
            echo "screen-sharing dev shell - run 'task' for available commands"
            echo "kmsgrab needs CAP_SYS_ADMIN: enable nix/screen-share.nix for the"
            echo "ffmpeg-kmsgrab wrapper, or run plain ffmpeg kmsgrab under sudo."

            # watch.Select reads this; mpv is the viewer for this shell. Unset it
            # to fall back to the in-code default, ffplay.
            export SCREENSHARE_VIEWER=mpv
          ''
          + pkgs.lib.optionalString pkgs.stdenv.isLinux ''
            export GST_PLUGIN_SYSTEM_PATH_1_0="${
              pkgs.lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstDeps
            }"
          ''
          + pkgs.lib.optionalString (pkgs.stdenv.isLinux && amfRuntime != [ ]) ''
            # ffmpeg dlopens libamfrt64.so.1 by soname, so the AMF runtime has to
            # be on the loader path and not merely in the shell closure. The
            # directory holds nothing but libamfrt64, so it shadows no system
            # library for the processes that inherit this.
            export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath amfRuntime}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
          '';
        };

        # Shell for nativegrid, the native grid binary. Kept out of the
        # default shell so the app's build inputs stay unchanged; gtk4 and
        # libadwaita exist only here. GStreamer core and base satisfy go-gst's
        # cgo pkg-config; the plugin path mirrors the default shell's so the
        # grid also runs from this shell directly, decoding included.
        devShells.nativegrid = pkgs.mkShell {
          packages =
            with pkgs;
            [
              go
              pkg-config
              gtk4
              libadwaita
              gobject-introspection
            ]
            ++ [
              gst_all_1.gstreamer
              gst_all_1.gst-plugins-base
            ];

          shellHook = ''
            export GST_PLUGIN_SYSTEM_PATH_1_0="${
              pkgs.lib.makeSearchPathOutput "lib" "lib/gstreamer-1.0" gstDeps
            }"
          '';
        };
      }
    )
    // {
      # Import from a host config to enable the privileged kmsgrab wrapper:
      #   imports = [ screen-sharing.nixosModules.screenShare ];
      #   programs.screenShare = { enable = true; user = "bjoern"; };
      nixosModules.screenShare = import ./nix/screen-share.nix;
      nixosModules.default = self.nixosModules.screenShare;
    };
}
