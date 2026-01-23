#!/bin/bash
# BV (Beads Viewer) Launcher for Chrote Dashboard
# Launches bv in the specified project directory
# USAGE: ttyd -W -a /usr/local/bin/bv-launch.sh
# -----------------------------------------------------------------------------

export LANG=en_US.UTF-8

# Get project path from first argument, default to /code
PROJECT_PATH="${1:-/code}"

# Change to project directory (fallback to /code if invalid)
cd "$PROJECT_PATH" 2>/dev/null || cd /code 2>/dev/null || cd ~

# Check if .beads directory exists in current location
if [ ! -d ".beads" ]; then
    echo "No .beads directory found in $PWD"
    echo "Starting bash shell instead. Run 'bv' manually in a beads project."
    exec bash -l
fi

# Launch bv (fall back to bash if bv not installed)
if command -v bv &> /dev/null; then
    exec bv
else
    echo "bv (beads_viewer) is not installed."
    echo "Install with: go install github.com/Dicklesworthstone/beads_viewer@latest"
    echo ""
    echo "Starting bash shell instead."
    exec bash -l
fi
