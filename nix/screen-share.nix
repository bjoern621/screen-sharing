# NixOS module for the screen-sharing project's privileged kmsgrab capture path.
#
# kmsgrab reads the raw KMS scanout framebuffer.
# The kernel gates that read behind CAP_SYS_ADMIN, so an unprivileged ffmpeg fails
# with "Failed to open DRM device" even when the /dev/dri node itself is readable.
#
# This module grants CAP_SYS_ADMIN to one dedicated wrapper, /run/wrappers/bin/ffmpeg-kmsgrab,
# and restricts execution to the video group.
# ffmpeg on PATH keeps no added privilege.
# The wrapper's path is exported as SCREENSHARE_FFMPEG_KMSGRAB so the app runs it for
# kmsgrab capture while every other path stays on the unprivileged ffmpeg.
#
# CAP_SYS_ADMIN is close to full root, and the wrapper is a complete ffmpeg that also
# parses untrusted media, so any video-group member can run arbitrary ffmpeg with that
# capability. Enable it only where every member of the video group is trusted.
# On Wayland the unprivileged PipeWire portal or wlroots screencopy path avoids the
# capability entirely and is preferred where the compositor supports it.
#
# Example capture after enabling and rebuilding (device index is host-specific, list
# CRTCs with `modetest`):
#
#   ffmpeg-kmsgrab -hide_banner -device /dev/dri/card1 -f kmsgrab -framerate 60 -i - \
#     -vf 'hwmap=derive_device=vaapi,hwdownload,format=bgr0' \
#     -c:v libx264 -preset veryfast -tune zerolatency \
#     -b:v 150M -pix_fmt yuv444p -color_range pc -g 120 \
#     -f mpegts "srt://127.0.0.1:8890?streamid=publish:nixos&pkt_size=1316&latency=60000&sndbuf=150000000&ffs=150000000"
#
# The hwmap step handles a scanout framebuffer that is GPU tiled or compressed (a nonzero
# DRM format modifier), which a bare hwdownload fails to map with EINVAL. The mapping
# device is GPU-dependent: VAAPI on Intel and AMD as shown, Vulkan on NVIDIA and elsewhere.
# The app selects it per capture; see ffmpeg.DrmMaps.
# The SRT URL is quoted because its unquoted ampersands would background the shell job.

{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.screenShare;

  # The ffmpeg build the wrapper exposes, with the AMF runtime placed where a
  # capability-bearing binary can still find it.
  #
  # ffmpeg reaches AMD's encoders by dlopening libamfrt64.so.1 from libavutil, and nothing
  # links that library, so it is found through a search path rather than a recorded
  # dependency. The wrapper carries file capabilities, which puts it in glibc's
  # secure-execution mode, and there LD_LIBRARY_PATH is ignored: the variable the dev shell
  # and every packaging layer set to deliver the runtime does not reach the loader. Ordinary
  # variables do survive, so this affects AMF alone and not the oneVPL runtime, which is
  # located through ONEVPL_SEARCH_PATH.
  #
  # Untreated it is a wrong answer rather than a missing encoder. encoders.Detect probes the
  # unprivileged ffmpeg, which does find the runtime, so the settings form offers h264_amf,
  # hevc_amf and av1_amf, and a kmsgrab publish with one of them dies at launch with
  # "DLL libamfrt64.so.1 failed to open".
  #
  # The runtime therefore goes on libavutil's own RUNPATH, which the loader honours in
  # secure-execution mode. dlopen searches the RUNPATH of the object that calls it, so the
  # entry belongs on that library and not on the executable. Patching one copied library is
  # what keeps this a copy rather than an ffmpeg rebuild: the executable already lists the
  # original library directory, and prepending the copy resolves the soname to it while every
  # other library still comes from the original build.
  kmsgrabFFmpeg =
    if cfg.amf == null then
      cfg.ffmpeg
    else
      pkgs.runCommand "ffmpeg-kmsgrab-${cfg.ffmpeg.version}"
        {
          nativeBuildInputs = [ pkgs.patchelf ];
        }
        ''
          mkdir -p $out/bin $out/lib

          real=$(readlink -f ${lib.getLib cfg.ffmpeg}/lib/libavutil.so)
          base=$(basename "$real")
          cp "$real" "$out/lib/$base"
          chmod u+w "$out/lib/$base"
          patchelf --add-rpath ${lib.makeLibraryPath [ cfg.amf ]} "$out/lib/$base"
          # The dependants of libavutil ask for its soname, so the copy answers to that name too.
          soname=$(patchelf --print-soname "$out/lib/$base")
          [ "$soname" = "$base" ] || ln -s "$base" "$out/lib/$soname"

          cp ${lib.getBin cfg.ffmpeg}/bin/ffmpeg $out/bin/ffmpeg
          chmod u+w $out/bin/ffmpeg
          patchelf --set-rpath "$out/lib:$(patchelf --print-rpath $out/bin/ffmpeg)" $out/bin/ffmpeg
        '';
in
{
  options.programs.screenShare = {
    enable = lib.mkEnableOption "the screen-sharing kmsgrab capture path (grants CAP_SYS_ADMIN to a dedicated ffmpeg wrapper)";

    user = lib.mkOption {
      type = lib.types.str;
      description = ''
        User added to the video group so it may execute the capability wrapper.
        The wrapper is mode 0750 root:video, so group membership is what authorizes it.
      '';
    };

    ffmpeg = lib.mkOption {
      type = lib.types.package;
      default = pkgs.ffmpeg-full;
      defaultText = lib.literalExpression "pkgs.ffmpeg-full";
      description = ''
        ffmpeg build exposed through the kmsgrab wrapper.
        Must include the kmsgrab input device, which ffmpeg-full provides.
      '';
    };

    amf = lib.mkOption {
      type = lib.types.nullOr lib.types.package;
      default = null;
      example = lib.literalExpression "pkgs.amf";
      description = ''
        AMD AMF runtime linked into the wrapper's libavutil, or null to leave the *_amf
        encoders unavailable under kmsgrab.

        A capability-bearing binary runs in glibc's secure-execution mode, where
        LD_LIBRARY_PATH is ignored, so a runtime delivered through the environment never
        reaches the loader. Naming the package here records it on libavutil's RUNPATH
        instead, which the loader does honour.

        A host running Mesa RADV requires AMF 1.4.37 or newer. Earlier releases request the
        pre-standard VK_AMD_video_encode_* device extensions that only AMD's proprietary
        Vulkan driver ever exposed, and ffmpeg dies with SIGSEGV inside
        AMFDeviceVulkanImpl::CreateDeviceAndFindQueues.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    # One capability-bearing copy of ffmpeg under /run/wrappers/bin.
    # The video group gate keeps the CAP_SYS_ADMIN blast radius to that group rather
    # than every local user.
    security.wrappers.ffmpeg-kmsgrab = {
      source = "${kmsgrabFFmpeg}/bin/ffmpeg";
      owner = "root";
      group = "video";
      permissions = "u+rx,g+rx,o-rwx";
      capabilities = "cap_sys_admin+ep";
    };

    # The DRM primary node is already reachable through the logind uaccess ACL on the
    # active seat, so this grants the wrapper gate, not raw node access.
    users.users.${cfg.user}.extraGroups = [ "video" ];

    # Point the app's kmsgrab capture at the capability wrapper by absolute path.
    # A session variable reaches a menu-launched GUI, which a login shell's PATH
    # export would not, so the app does not depend on /run/wrappers/bin being on
    # its inherited PATH.
    environment.sessionVariables.SCREENSHARE_FFMPEG_KMSGRAB = "${config.security.wrapperDir}/ffmpeg-kmsgrab";

    # Unprivileged userspace tools for capture and for inspecting the capture path.
    environment.systemPackages = with pkgs; [
      cfg.ffmpeg
      libva-utils # vainfo: list VAAPI encode entrypoints for the zero-copy path
      libdrm # modetest: enumerate CRTCs and planes to pick a kmsgrab device
      drm_info # human-readable dump of DRM connectors, planes, and formats
    ];
  };
}
