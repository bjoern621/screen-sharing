# The environment an isolated backend runs under, sourced by every script here.
#
# Four variables and every one is needed. The runtime directory moves the control socket, so this
# instance neither finds nor is found by the one a shell started. The config directory moves the
# settings, the presets, the portal token and the run logs. No synthetic publishers on boot leaves
# the relay paths free. And the Pulse socket is named outright, libpulse looking for it under the
# runtime directory this has just moved.
#
# The session is stated as x11 so the capture list offers the portal-free backends: a probe that
# captured through the portal would pop a consent picker on somebody's screen.
#
# SOAK_ROOT is where everything this run writes goes, and defaults beside the repository rather than
# inside it.
export SOAK_ROOT="${SOAK_ROOT:-$HOME/.cache/screenshare-soak}"
mkdir -p "$SOAK_ROOT"

export XDG_RUNTIME_DIR="$SOAK_ROOT/run"
export XDG_CONFIG_HOME="$SOAK_ROOT/config"
export SCREENSHARE_TEST_STREAMS=0
export XDG_SESSION_TYPE=x11
export DISPLAY="${DISPLAY:-:0}"
unset WAYLAND_DISPLAY
export PULSE_SERVER="${PULSE_SERVER:-unix:/run/user/$(id -u)/pulse/native}"
