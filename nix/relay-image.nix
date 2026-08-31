# The relay as a container image, for a Kubernetes deployment of it.
#
# The image carries deploy/mediamtx-groups.yml rather than expecting one mounted beside it.
# That file and the MediaMTX build it is written against are one decision,
# and an image tag pinning the binary while a ConfigMap elsewhere pins the config
# is two halves that drift apart on the next `moq*` key.
#
# The published images are the only thing MediaMTX offers here, and none of them carries a shell:
# the config's runOnRead hook is /bin/sh with curl in it,
# and a relay built on those images serves reads it never reports.
# busybox and curl are why this file exists.

{
  lib,
  dockerTools,
  mediamtx,
  busybox,
  curl,
  cacert,
  version ? lib.fileContents ../VERSION,
}:

dockerTools.buildLayeredImage {
  name = "screenshare-relay";
  tag = version;

  contents = [
    mediamtx
    busybox
    curl
    # The group service's JWKS endpoint is plain HTTP over loopback,
    # but curl in a certificate-less image fails any https:// a hook opens,
    # with a message about the store rather than the URL.
    cacert
  ];

  # The two paths the config names, and it names them absolutely, so neither moves.
  extraCommands = ''
    mkdir -p etc/mediamtx
    cp ${../deploy/mediamtx-groups.yml} etc/mediamtx/mediamtx.yml
    install -m 0555 ${../deploy/reconcile-on-read.sh} etc/mediamtx/reconcile-on-read.sh
  '';

  config = {
    # MediaMTX takes the config path as its one argument.
    # Left off it searches a list of locations,
    # and a copy landing in one of them would decide the deployment silently.
    Entrypoint = [
      "/bin/mediamtx"
      "/etc/mediamtx/mediamtx.yml"
    ];

    # Every listener the config binds is above 1024,
    # so the relay needs no capability and no account of its own.
    User = "65532:65532";

    # The MoQ pair is the one certificate reference the config states relatively,
    # and MediaMTX resolves it against the working directory.
    # Set so a deployment overriding no MTX_MOQSERVERCERT
    # draws its own pair somewhere writable instead of failing to start.
    WorkingDir = "/tmp";
  };
}
