#!/usr/bin/env bash
#
# relay-api-token.sh - print a token granting the relay's own API.
#
#   sh scripts/relay-api-token.sh <relay host> [window]
#   sh scripts/relay-api-token.sh netcup-g12 2h
#
# A relay that authenticates grants its API, its metrics and its playback endpoints to
# nothing a group token carries (deploy/mediamtx-groups.yml), so reading it takes a
# credential signed with the group service's own key.
#
# The signing is done on the relay host over SSH and never here: that key signs every
# group's tokens too, so it stays on the machine it is on. What comes back is a token with
# the API action and nothing else, valid for the window given.
#
# The API also binds loopback and is not behind the reverse proxy, so a caller wanting to
# read it wants a tunnel beside this token:
#
#   task relay:tunnel
#
# The binary is read off the running unit because a NixOS deployment installs the service
# without putting it on any shell's path (modules/screenshare-groupd.nix in nixos-config).
set -euo pipefail

host=${1:?usage: relay-api-token.sh <relay host> [window]}
window=${2:-2h}

# The key file is the unit's own StateDirectory, which systemd puts under /var/lib/private
# for a DynamicUser service, and root is the only account that reads it.
ssh "$host" "sudo \$(systemctl show -p ExecStart --value groupd | tr ';' '\n' | grep -o 'path=[^ ]*' | cut -d= -f2) \
    -api-token $window -key /var/lib/private/groupd/signing-key.pem"
