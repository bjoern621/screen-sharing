#!/usr/bin/env bash
# Ends the loops, the probes and the backend of this root, and nothing else.
#
# Everything is matched on SOAK_ROOT rather than on a process name:
# a second root can be running its own pair,
# and a shell's backend is never one of these whatever it is called.
set -u
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$here/env.sh"

# The loops read this before each run, so they end on their own even if the kill below misses one.
touch "$SOAK_ROOT/stop"

me=$$
for pid in $(pgrep -f "$SOAK_ROOT"); do
  [ "$pid" = "$me" ] && continue
  case "$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null)" in
    *supervise.sh*|*form-loop.sh*) kill "$pid" 2>/dev/null ;;
  esac
done
sleep 2

# The probes, named by the findings file they were told to write, which carries this root.
for pid in $(pgrep -x soak); do
  case "$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null)" in
    *"$SOAK_ROOT"*) kill "$pid" 2>/dev/null ;;
  esac
done
sleep 3

if pid=$("$here/findpid.sh"); then
  kill "$pid" 2>/dev/null
fi

echo "stopped, findings left in $SOAK_ROOT"
