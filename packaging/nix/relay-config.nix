# deploy/mediamtx-groups.yml, checked against the MediaMTX build that reads it.
#
# MediaMTX refuses a config carrying a key it does not know, and it does that at startup,
# so in a deployment the answer arrives as a crashlooping pod rather than a failed build.
# The relay image draws the file from here, and an unchecked copy therefore reaches no image.

{
  runCommand,
  mediamtx,
}:

let
  config = ../../deploy/mediamtx-groups.yml;
in
runCommand "mediamtx-groups.yml" { } ''
  # MediaMTX draws the MoQ pair beside whatever it runs in, and the store is read-only.
  cd "$(mktemp -d)"

  # It runs until something stops it,
  # and the certificates the config names belong to a deployment rather than to this sandbox,
  # so it exits on the first one it cannot open.
  # The exit status answers nothing, and the line naming a parsed config is what is read.
  timeout 20 ${mediamtx}/bin/mediamtx ${config} > load.log 2>&1 || true

  grep -q 'configuration loaded' load.log || {
    cat load.log >&2
    exit 1
  }

  cp ${config} "$out"
''
