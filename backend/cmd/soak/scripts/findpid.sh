#!/usr/bin/env bash
# The backend of this run, matched on the runtime directory it serves its socket in.
#
# Ordering would not do it: the user's own backend can be the newer process, and a probe that watched
# or killed that one would be reaching into the app this run exists to stay out of.
for p in $(pgrep -x backend); do
  if tr '\0' '\n' < "/proc/$p/environ" 2>/dev/null | grep -qx "XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR"; then
    echo "$p"
    exit 0
  fi
done
exit 1
