#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal Launcher for AgentArena
# Ensures ttyd sessions connect to the correct tmux socket
# USAGE: ttyd -W -a /usr/local/bin/terminal-launch.sh
# -----------------------------------------------------------------------------

# Use explicit socket path from systemd or default to /run/tmux/chrote
export TMUX_TMPDIR=${TMUX_TMPDIR:-/run/tmux/chrote}
export LANG=en_US.UTF-8
# REASON: cd to preferred dir is optional, fallthrough is intentional
cd /code 2>/dev/null || cd ~

SESSION="$1"

case "$SESSION" in
  gc:*)
    GC_SESSION="${SESSION#gc:}"
    if [ -z "$GC_SESSION" ]; then
      echo "terminal-launch: gc target requires a session id" >&2
      exit 2
    fi
    GC_CITY_DIR="${CHROTE_GASCITY_CITY_DIR:-$HOME/gascity}"
    if [ ! -d "$GC_CITY_DIR/.gc" ]; then
      echo "terminal-launch: Gas City not found at $GC_CITY_DIR (set CHROTE_GASCITY_CITY_DIR)" >&2
      exit 2
    fi
    case "${CHROTE_GASCITY_GC_PATH:-}" in
      off|OFF) ;;
      "")
        [ -x /home/linuxbrew/.linuxbrew/bin/tmux ] && export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"
        ;;
      *)
        export PATH="${CHROTE_GASCITY_GC_PATH}:$PATH"
        ;;
    esac
    # Leave TMUX_TMPDIR intact so Gas City attaches to the same tmux socket
    # namespace as the supervisor-managed sessions. TMUX itself must be cleared
    # because ttyd launches from outside an attached tmux client.
    unset TMUX
    exec gc --city "$GC_CITY_DIR" session attach "$GC_SESSION"
    ;;
esac

# Check if session exists and attach, otherwise start fresh shell
# REASON: tmux has-session tests existence; stderr is noise, not an error
if [ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null; then
    exec tmux attach-session -t "$SESSION"
else
    # If session doesn't exist, just drop to shell (don't error out)
    exec bash -l
fi
