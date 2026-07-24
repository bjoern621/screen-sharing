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
        pkgs = import nixpkgs { inherit system; };
        linuxDeps = with pkgs; [
          gtk3
          webkitgtk_4_1 # Wails v2 Linux backend (build with tag webkit2_41)
          gtk4 # gridviewer binary (gotk4); separate process, never linked with gtk3
          gobject-introspection # gotk4's gerror package links it
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
        # GStreamer runs on both sides of the wire. Publish: pipewiresrc reads the
        # xdg-desktop-portal ScreenCast node, the rest encodes and ships over SRT
        # or RTSP. Watch: the native grid wall decodes every stream through
        # decodebin and composites them into one window. gst-launch-1.0 finds
        # these plugins through GST_PLUGIN_SYSTEM_PATH_1_0 (set in the shellHook).
        gstDeps =
          with pkgs.gst_all_1;
          [
            gstreamer # gst-launch-1.0
            gst-plugins-base # videoconvert, compositor, textoverlay
            gst-plugins-good # rtspsrc
            gst-plugins-bad # mpegtsmux, srtsink/srtsrc, h264parse/h265parse, nvcodec
            gst-plugins-ugly # x264enc
            gst-libav # avdec_h264/avdec_h265: the only decoders covering 4:4:4/RGB (RExt)
            gst-rtsp-server # rtspclientsink
            gst-plugins-rs # gtk4paintablesink (gridviewer video sink)
          ]
          ++ [ pkgs.pipewire ]; # pipewiresrc gst plugin
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
          ''
          + pkgs.lib.optionalString pkgs.stdenv.isLinux ''
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
