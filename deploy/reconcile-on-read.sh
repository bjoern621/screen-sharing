#!/bin/sh
# Reports a starting connection to the group service,
# which closes it where no live member of that group holds it.
#
# The relay checks a token at the handshake and not again,
# so a member whose presence lapsed still holds an unexpired token
# and opens a connection the relay is happy to serve.
# This is what closes it: the connection announces itself,
# the service runs that group's presence leases against the relay,
# and anything no live member holds goes.
#
# Both runOnRead and runOnAvailable, a publisher who left otherwise standing
# until a sweep or another member's poll reaches them (deploy/mediamtx-groups.yml).
# MediaMTX sets MTX_PATH for each, which is the whole of what this reports.
# A group with no live member is left alone, save for a member released inside the token window,
# so this is a no-op on every relay nobody has stated presence at,
# and a path outside any group is refused by the service rather than filtered here.
set -eu

[ -n "${MTX_PATH:-}" ] || exit 0

# Bounded and quiet: the connection is already open by the time this runs,
# so a service that is slow or down must not hold a hook open.
# Enforcement is missed rather than delayed, and the next connection reports again.
exec curl -sS -m 2 -o /dev/null \
	-X POST \
	-H 'Content-Type: application/json' \
	-d "{\"path\":\"${MTX_PATH}\"}" \
	"${SCREENSHARE_GROUP_SERVICE:-http://127.0.0.1:9443}/reconcile"
