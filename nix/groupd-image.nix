# The group service as a container image, for a Kubernetes deployment of it.
#
# Nothing but the binary. Its listen address, the relay it reads the stream list from, and
# where it keeps its signing key are all the deployment's, so they are arguments rather than
# anything baked in here.

{
  lib,
  dockerTools,
  screenshare-groupd,
  version ? lib.fileContents ../VERSION,
}:

dockerTools.buildLayeredImage {
  name = "screenshare-groupd";
  tag = version;

  contents = [ screenshare-groupd ];

  config = {
    Entrypoint = [ "/bin/groupd" ];
    # Answers JSON on a port above 1024 and writes one file, into a volume the deployment
    # gives it.
    User = "65532:65532";
  };
}
