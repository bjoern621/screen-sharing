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
          overlays = [
            (final: prev: {
              webkitgtk_4_1 = prev.webkitgtk_4_1.override { enableExperimental = true; };
            })
          ];
        };
        linuxDeps = with pkgs; [
          gtk3
          webkitgtk_4_1 # Wails v2 Linux backend (build with tag webkit2_41)
          pkg-config
          xrandr # X11 monitor enumeration (display pkg listX11)
          # The native grid renders its vendored Tabler SVGs through GdkTexture,
          # which needs the librsvg gdk-pixbuf loader. librsvg's setup hook
          # exports GDK_PIXBUF_MODULE_FILE, which the grid inherits from the
          # app that spawns it; png/jpeg/gif stay gdk-pixbuf builtins, so the
          # override costs the GTK3/WebKit processes nothing.
          librsvg
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
            gst-plugins-good # pulsesrc (desktop audio), vpx (vp9enc), rtspsrc, progressreport
            gst-plugins-bad # mpegtsmux, srtsink/srtsrc, h264parse/h265parse, nvcodec, opusenc
            gst-plugins-ugly # x264enc
            gst-rtsp-server # rtspclientsink
            gst-libav # avdec_h264/avdec_h265: the only decoders for H.264 4:4:4 and HEVC RExt (RGB)
            gst-plugins-rs # gtk4paintablesink (native grid video sink)
          ]
          ++ [
            pkgs.pipewire # pipewiresrc gst plugin
            pkgs.libnice # nice elements: ICE for webrtcbin, i.e. WHEP in the webview
          ];
      in
      {
        devShells.default = pkgs.mkShell {
          packages =
            with pkgs;
            [
              go
              nodejs_22
              wails
              ffmpeg-full # includes x11grab/kmsgrab + ffplay
              mpv # single-stream viewer, selected by SCREENSHARE_VIEWER below
              mediamtx # the relay, run natively: `mediamtx mediamtx.yml`
              go-task # task runner, see Taskfile.yml
              # Nix
              nil
              nixfmt
            ]
            ++ pkgs.lib.optionals pkgs.stdenv.isLinux (linuxDeps ++ linuxCaptureDeps ++ gstDeps);

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
              librsvg # Tabler SVGs via GdkTexture; the setup hook exports GDK_PIXBUF_MODULE_FILE
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
