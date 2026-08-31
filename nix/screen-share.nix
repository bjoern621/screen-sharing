# NixOS module for the screen-sharing project's privileged kmsgrab capture path.
#
# kmsgrab reads the raw KMS scanout framebuffer,
# and the kernel gates that read behind CAP_SYS_ADMIN:
# an unprivileged ffmpeg fails with "Failed to open DRM device"
# even where the /dev/dri node itself is readable.
#
# The capability goes on one dedicated wrapper, /run/wrappers/bin/ffmpeg-kmsgrab,
# executable by the video group, and ffmpeg on PATH gains nothing.
# Its path is exported as SCREENSHARE_FFMPEG_KMSGRAB, which the app runs for kmsgrab capture,
# every other path staying on the unprivileged ffmpeg.
#
# CAP_SYS_ADMIN is close to full root,
# and the wrapper is a complete ffmpeg that also parses untrusted media,
# so any video-group member can run arbitrary ffmpeg with that capability.
# Enable this only where every member of that group is trusted.
# A Wayland compositor serving the PipeWire portal or wlroots screencopy needs no capability at all,
# and that path is preferred where it exists.
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
  cfg = config.programs.screenShare;

  # The ffmpeg build the wrapper exposes,
  # with the AMF runtime placed where a capability-bearing binary can still find it.
  #
  # libavutil dlopens AMD's libamfrt64.so.1 by soname and nothing links it,
  # so it is reached through a search path rather than through a recorded dependency.
  # A wrapper carrying file capabilities runs in glibc's secure-execution mode,
  # where LD_LIBRARY_PATH is ignored,
  # so the variable the dev shell and every packaging layer set to deliver that runtime
  # never reaches the loader.
  # Ordinary variables survive, so this affects AMF alone
  # and not the oneVPL runtime behind QSV, located through ONEVPL_SEARCH_PATH.
  #
  # Untreated it is a wrong answer rather than a missing encoder:
  # encoders.Detect probes the unprivileged ffmpeg, which does find the runtime,
  # so the settings form offers h264_amf, hevc_amf and av1_amf,
  # and a kmsgrab publish with one of them dies at launch
  # with "DLL libamfrt64.so.1 failed to open".
  #
  # The runtime therefore goes on libavutil's own RUNPATH,
  # which the loader honours in secure-execution mode,
  # and dlopen searches the RUNPATH of the object that calls it,
  # so the entry belongs on that library and not on the executable.
  # Patching one copied library keeps this a copy rather than an ffmpeg rebuild:
  # the executable already lists the original library directory,
  # and prepending the copy resolves the soname to it
  # while every other library still comes from the original build.
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
    # Mode 0750 root:video, so group membership is what authorizes it,
    # and CAP_SYS_ADMIN reaches that group rather than every local user.
    security.wrappers.ffmpeg-kmsgrab = {
      source = "${kmsgrabFFmpeg}/bin/ffmpeg";
      owner = "root";
      group = "video";
      permissions = "u+rx,g+rx,o-rwx";
      capabilities = "cap_sys_admin+ep";
    };

    # This grants the wrapper gate and not raw node access:
    # the DRM primary node is reachable through the logind uaccess ACL on the active seat.
    users.users.${cfg.user}.extraGroups = [ "video" ];

    # The wrapper by absolute path,
    # so the app depends on nothing having put /run/wrappers/bin on its inherited PATH.
    # A session variable reaches a menu-launched GUI, which a login shell's PATH export does not.
    environment.sessionVariables.SCREENSHARE_FFMPEG_KMSGRAB = "${config.security.wrapperDir}/ffmpeg-kmsgrab";

    # The unprivileged ffmpeg every other capture path runs, plus what inspecting this one takes.
    environment.systemPackages = with pkgs; [
      cfg.ffmpeg
      libva-utils # vainfo: VAAPI encode entrypoints, for the zero-copy path
      libdrm # modetest: CRTCs and planes, to pick a kmsgrab device
      drm_info # DRM connectors, planes and formats
    ];
  };
}
