# The Discord manager as a container image, for a Kubernetes deployment of it.
#
# Nothing but the binary.
# The bot token and the OAuth application secrets arrive in the environment,
# the listen address and the link store path as arguments,
# so nothing is baked in here.

{
  dockerTools,
  screenshare-discordd,
  version,
}:

dockerTools.buildLayeredImage {
  name = "screenshare-discordd";
  tag = version;

  contents = [ screenshare-discordd ];

  config = {
    Entrypoint = [ "/bin/discordd" ];
    # Answers JSON on a port above 1024,
    # and writes one file into a volume the deployment gives it.
    User = "65532:65532";
  };
}
