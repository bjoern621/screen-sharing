#!/usr/bin/env bash
# The measuring modes, one at a time, until a file named stop appears in SOAK_ROOT.
#
# One at a time because they compete for the same silicon: a run that overlapped them would report
# the contention as a finding. The capture alternates between the two engines, each instrumenting
# what it runs differently, so a run that never left one of them would report half the product.
set -u
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$here/env.sh"

cycle=0
while [ ! -f "$SOAK_ROOT/stop" ]; do
  cycle=$((cycle + 1))
  for spec in "encode 900" "publish 900" "multi 600"; do
    set -- $spec
    mode=$1; secs=$2
    [ -f "$SOAK_ROOT/stop" ] && break

    # A backend that died is started again: the probe watching one is the point of the run.
    if ! pgrep -x backend | grep -qx "$(cat "$SOAK_ROOT/backend.pid" 2>/dev/null)"; then
      echo "$(date -Is) backend gone, starting one" >> "$SOAK_ROOT/supervise.log"
      nohup "$SOAK_ROOT/bin/backend" > "$SOAK_ROOT/backend-$(date +%s).log" 2>&1 &
      sleep 5
      "$here/findpid.sh" > "$SOAK_ROOT/backend.pid"
    fi

    pid=$(cat "$SOAK_ROOT/backend.pid")
    seed=$((cycle * 1000 + RANDOM))
    if [ $((cycle % 2)) -eq 0 ]; then capture=ximagesrc; else capture=x11grab; fi
    echo "$(date -Is) cycle $cycle: $mode for ${secs}s on $capture, seed $seed, backend $pid" >> "$SOAK_ROOT/supervise.log"

    "$SOAK_ROOT/bin/soak" -mode "$mode" -for "${secs}s" -seed "$seed" -backend-pid "$pid" \
      -publish-for 20s -ramp 0,1,3,6,9 -capture "$capture" \
      -out "$SOAK_ROOT/$mode.jsonl" >> "$SOAK_ROOT/$mode.out" 2>&1
  done
done
echo "$(date -Is) stopped" >> "$SOAK_ROOT/supervise.log"
