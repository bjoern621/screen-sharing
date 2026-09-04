# NixOS module for MirrorMe's privileged kmsgrab capture path.
#
# The capability and nothing else.
# The app, ffmpeg and the inspection tools install without root,
# so they belong wherever the rest of a user's packages are declared,
# and the app carries its own ffmpeg on PATH already (packaging/nix/package.nix).
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
  cfg = config.programs.mirrorme.kmsgrab;

  kmsgrabFFmpeg = pkgs.callPackage ./kmsgrab-ffmpeg.nix { inherit (cfg) ffmpeg amf; };
in
{
  options.programs.mirrorme.kmsgrab = {
    enable = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        Whether to configure a setcap wrapper letting MirrorMe capture the DRM scanout
        framebuffer. To use it, add your user to the `mirrorme` group. Installing the
        app itself is separate and needs no privilege.

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
        Exposed through the wrapper. Must include the kmsgrab input device, which
        `ffmpeg-full` provides.
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

    # One capability-bearing copy of ffmpeg under /run/wrappers/bin.
    # Mode 0510 root:mirrorme, the activation script chmodding from 0000,
    # so group membership is what authorizes it and CAP_SYS_ADMIN reaches no other local user.
    #
    # A name of its own rather than shadowing ffmpeg on PATH,
    # which would hand the capability to every capture path that needs none.
    security.wrappers.ffmpeg-kmsgrab = {
      source = "${kmsgrabFFmpeg}/bin/ffmpeg";
      owner = "root";
      group = "mirrorme";
      permissions = "u+rx,g+x";
      capabilities = "cap_sys_admin+ep";
    };

    # The wrapper by absolute path,
    # so the app depends on nothing having put /run/wrappers/bin on its inherited PATH.
    # A session variable reaches a menu-launched GUI, which a login shell's PATH export does not.
    environment.sessionVariables.MIRRORME_FFMPEG_KMSGRAB = "${config.security.wrapperDir}/ffmpeg-kmsgrab";
  };
}
