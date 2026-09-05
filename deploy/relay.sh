#!/usr/bin/env bash
# Runs the relay and the group service beside it, against deploy/mediamtx-groups.yml.
#
# The relay verifies every connection against the key set the group service publishes,
# so a relay started on its own refuses every publisher:
# the two come up together or neither serves anything.
#
# A machine with no deployment certificate and no hook paths takes both as environment overrides,
# the file itself being the one every relay reads (deploy/mediamtx-groups.yml).
#
# Foreground, and Ctrl+C ends both.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# Everything drawn for a development relay, none of it committed:
# the certificate the TLS listeners hold, the key the group service signs with,
# and the MoQ pair MediaMTX draws in its working directory.
dev="$root/dev-relay"
cert="$dev/cert.pem"
key="$dev/key.pem"

mkdir -p "$dev"

# Drawn where absent, so a second run keeps the certificate whoever trusted it once already holds.
# localhost and the loopback addresses, those being the names a relay on this machine is reached by.
if [ ! -f "$cert" ] || [ ! -f "$key" ]; then
	echo "drawing a development certificate into $dev"
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -sha256 -days 365 \
		-subj "/CN=localhost" \
		-addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1" \
		-keyout "$key" -out "$cert"
fi

# Built rather than `go run`, which puts a second process between this script and the service,
# and leaves it running when the signal reaches the wrapper alone.
(cd "$root/backend" && go build -o "$root/bin/groupd" ./cmd/groupd)

# The signing key is stored, so tokens issued before a restart are still verified after one:
# the relay caches the key set it fetched,
# and a service drawing a new key on every start would refuse every connection
# until that cache turned over.
# Reports land beside the other drawn state, so a development relay takes them like a deployment.
"$root/bin/groupd" -key "$dev/signing-key.pem" -reports "$dev/reports" &
groupd=$!
trap 'kill "$groupd" 2>/dev/null || true' EXIT

# From the relay's own directory, because MediaMTX draws the MoQ pair beside whatever it runs in.
cd "$dev"
MTX_RTSPSERVERCERT="$cert" MTX_RTSPSERVERKEY="$key" \
	MTX_RTMPSERVERCERT="$cert" MTX_RTMPSERVERKEY="$key" \
	MTX_PATHDEFAULTS_RUNONREAD="$root/deploy/reconcile-on-read.sh" \
	mediamtx "$root/deploy/mediamtx-groups.yml"
