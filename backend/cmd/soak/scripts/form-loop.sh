#!/usr/bin/env bash
# The form walk, beside the measuring modes rather than in turn with them: it costs no silicon, so it
# contends with nothing they measure.
set -u
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$here/env.sh"

cycle=0
while [ ! -f "$SOAK_ROOT/stop" ]; do
  cycle=$((cycle + 1))
  "$SOAK_ROOT/bin/soak" -mode form -for 1800s -seed $((cycle * 7919 + RANDOM)) \
    -backend-pid "$(cat "$SOAK_ROOT/backend.pid")" \
    -out "$SOAK_ROOT/form.jsonl" >> "$SOAK_ROOT/form.out" 2>&1
done
