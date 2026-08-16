#!/bin/sh
# Reports a starting read to the group service, which closes it where the member holding it is not on
# that group's roster.
#
# The relay checks a token at the handshake and not again, so a member who left and still holds an
# unexpired token opens a connection the relay is happy to serve. This is what closes it: the read
# announces itself, the service runs that group's roster against the relay, and anything no member
# holds goes.
#
# MediaMTX sets MTX_PATH for runOnRead and runOnUnread (backend/internal/roster).
# A group nobody stated a roster for is left alone, so this is a no-op on every relay that serves no
# rosters, and a path outside any group is refused by the service rather than filtered here.
set -eu

[ -n "${MTX_PATH:-}" ] || exit 0

# Bounded and quiet: a read is already open by the time this runs, so a service that is slow or down
# must not hold a hook open. Enforcement is missed rather than delayed, and the next read reports
# again.
exec curl -sS -m 2 -o /dev/null \
	-X POST \
	-H 'Content-Type: application/json' \
	-d "{\"path\":\"${MTX_PATH}\"}" \
	"${SCREENSHARE_GROUP_SERVICE:-http://127.0.0.1:9443}/reconcile"
