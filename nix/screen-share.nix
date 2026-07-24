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
  };

  config = lib.mkIf cfg.enable {
    # One capability-bearing copy of ffmpeg under /run/wrappers/bin.
    # The video group gate keeps the CAP_SYS_ADMIN blast radius to that group rather
    # than every local user.
    security.wrappers.ffmpeg-kmsgrab = {
      source = "${cfg.ffmpeg}/bin/ffmpeg";
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
