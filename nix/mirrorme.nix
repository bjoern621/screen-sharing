# NixOS module for MirrorMe's system-level capture support.
#
# Two decisions, taken separately.
# `enable` installs the tools a capture path needs and creates the mirrorme group, granting nothing.
# `kmsgrab.enable` puts CAP_SYS_ADMIN on a dedicated ffmpeg wrapper for that group,
# which is what the DRM scanout path needs and what nothing else here does.
#
# kmsgrab reads the raw KMS scanout framebuffer,
# and the kernel gates that read behind CAP_SYS_ADMIN:
# an unprivileged ffmpeg fails with "Failed to open DRM device"
# even where the /dev/dri node itself is readable.
#
# A scanout framebuffer can be GPU tiled or compressed, carrying a nonzero DRM format modifier,
# which a bare hwdownload fails to map with EINVAL:
# a capture maps it through hwmap first, on a device the GPU decides
# (VAAPI on Intel and AMD, Vulkan elsewhere).
# The app selects that device per capture (ffmpeg.DrmMaps).

{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.mirrorme;

  kmsgrabFFmpeg = pkgs.callPackage ./kmsgrab-ffmpeg.nix { inherit (cfg) ffmpeg amf; };
in
{
  imports = [
    (lib.mkRemovedOptionModule [ "programs" "mirrorme" "user" ] ''
      Add the user to the mirrorme group instead:
      users.users.<name>.extraGroups = [ "mirrorme" ];
    '')
  ];

  options.programs.mirrorme = {
    enable = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        Whether to install the tools MirrorMe's capture paths need and create the
        'mirrorme' group. Nothing installed here is privileged. Reaching the DRM
        scanout framebuffer takes the separate `kmsgrab.enable` option below.
      '';
    };

    kmsgrab.enable = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        Whether to allow users in the 'mirrorme' group to capture the DRM scanout
        framebuffer. This configures a `CAP_SYS_ADMIN` wrapper around
        {option}`programs.mirrorme.ffmpeg` under {file}`/run/wrappers/bin` and points
        the app at it. Add a user with
        `users.users.<name>.extraGroups = [ "mirrorme" ];`.

        `CAP_SYS_ADMIN` is close to full root, and the wrapper is a complete ffmpeg
        that also parses untrusted media, so any member of that group can run
        arbitrary ffmpeg with that capability. Turn this on only where every member
        is trusted.

        A Wayland compositor serving the PipeWire portal or wlroots screencopy
        captures the desktop with no capability at all, and the app prefers that path
        where it exists.
      '';
    };

    ffmpeg = lib.mkPackageOption pkgs "ffmpeg-full" {
      extraDescription = ''
        Exposed through the kmsgrab wrapper and installed unwrapped for every other
        capture path. Must include the kmsgrab input device, which `ffmpeg-full`
        provides.
      '';
    };

    amf = lib.mkPackageOption pkgs "amf" {
      nullable = true;
      default = null;
      extraDescription = ''
        Linked into the wrapper's `libavutil`, or null to leave the `*_amf` encoders
        unavailable under kmsgrab. A capability-bearing binary runs in glibc's
        secure-execution mode, where `LD_LIBRARY_PATH` is ignored, so a runtime
        delivered through the environment never reaches the loader. Naming the
        package here records it on `libavutil`'s `RUNPATH` instead, which the loader
        does honour.

        A host running Mesa RADV requires AMF 1.4.37 or newer. Earlier releases
        request the pre-standard `VK_AMD_video_encode_*` device extensions that only
        AMD's proprietary Vulkan driver ever exposed, and ffmpeg dies with SIGSEGV
        inside `AMFDeviceVulkanImpl::CreateDeviceAndFindQueues`.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    # The gate the wrapper is group-owned by, empty of members until a host names one.
    # A group of its own rather than video:
    # video carries the machine's GPU users, a wider set than the one trusted with the capability.
    users.groups.mirrorme = { };

    # The unprivileged ffmpeg every other capture path runs, plus what inspecting this one takes.
    environment.systemPackages = [
      cfg.ffmpeg
      pkgs.libva-utils # vainfo: VAAPI encode entrypoints, for the zero-copy path
      pkgs.libdrm # modetest: CRTCs and planes, to pick a kmsgrab device
      pkgs.drm_info # DRM connectors, planes and formats
    ];

    # One capability-bearing copy of ffmpeg under /run/wrappers/bin.
    # Mode 0510 root:mirrorme, the activation script chmodding from 0000,
    # so group membership is what authorizes it and CAP_SYS_ADMIN reaches no other local user.
    security.wrappers.ffmpeg-kmsgrab = lib.mkIf cfg.kmsgrab.enable {
      source = "${kmsgrabFFmpeg}/bin/ffmpeg";
      owner = "root";
      group = "mirrorme";
      permissions = "u+rx,g+x";
      capabilities = "cap_sys_admin+ep";
    };

    # The wrapper by absolute path,
    # so the app depends on nothing having put /run/wrappers/bin on its inherited PATH.
    # A session variable reaches a menu-launched GUI, which a login shell's PATH export does not.
    environment.sessionVariables = lib.mkIf cfg.kmsgrab.enable {
      MIRRORME_FFMPEG_KMSGRAB = "${config.security.wrapperDir}/ffmpeg-kmsgrab";
    };
  };
}
