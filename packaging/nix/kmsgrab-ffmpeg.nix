# The ffmpeg the kmsgrab capability wrapper exposes,
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

{
  lib,
  runCommand,
  patchelf,
  ffmpeg,
  amf,
}:

if amf == null then
  ffmpeg
else
  runCommand "ffmpeg-kmsgrab-${ffmpeg.version}"
    {
      nativeBuildInputs = [ patchelf ];
    }
    ''
      mkdir -p $out/bin $out/lib

      real=$(readlink -f ${lib.getLib ffmpeg}/lib/libavutil.so)
      base=$(basename "$real")
      cp "$real" "$out/lib/$base"
      chmod u+w "$out/lib/$base"
      patchelf --add-rpath ${lib.makeLibraryPath [ amf ]} "$out/lib/$base"
      # The dependants of libavutil ask for its soname, so the copy answers to that name too.
      soname=$(patchelf --print-soname "$out/lib/$base")
      [ "$soname" = "$base" ] || ln -s "$base" "$out/lib/$soname"

      cp ${lib.getBin ffmpeg}/bin/ffmpeg $out/bin/ffmpeg
      chmod u+w $out/bin/ffmpeg
      patchelf --set-rpath "$out/lib:$(patchelf --print-rpath $out/bin/ffmpeg)" $out/bin/ffmpeg
    ''
