# The reverse proxy as a container image, for a Kubernetes deployment of the relay.
#
# It carries deploy/Caddyfile for the reason the relay image carries the relay's config:
# the routing table is one file,
# and a second copy of it in a cluster repository is a copy nobody reads against this one.
#
# What it does not carry is ACME.
# On a host this proxy is the only client that could hold a certificate, so it issues one;
# in the cluster cert-manager does,
# and the proxy is reached over a name another terminator answers for.
# The deployment sets SCREENSHARE_DOMAIN to a bare port for that,
# a Caddy site address with no scheme and so no automatic HTTPS.

{
  dockerTools,
  caddy,
  version,
}:

dockerTools.buildLayeredImage {
  name = "screenshare-proxy";
  tag = version;

  contents = [ caddy ];

  extraCommands = ''
    mkdir -p etc/caddy
    cp ${../../deploy/Caddyfile} etc/caddy/Caddyfile
  '';

  config = {
    Entrypoint = [
      "/bin/caddy"
      "run"
      "--config"
      "/etc/caddy/Caddyfile"
      "--adapter"
      "caddyfile"
    ];
    User = "65532:65532";
  };
}
