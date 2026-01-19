#!/bin/bash
# CHROTE Bootstrap - Simple wrapper for setup script
# Strips CRLF and executes setup-wsl.sh
#
# Usage: wsl -d Ubuntu-24.04 -u root -e bash /path/to/bootstrap.sh
# Or just use: .\Chrote-Toggle.ps1 -Setup

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHROTE_DIR="$(dirname "$SCRIPT_DIR")"
SETUP_SCRIPT="$SCRIPT_DIR/setup-wsl.sh"

[ "$EUID" -ne 0 ] && { echo "Error: Must run as root. Use: .\\Chrote-Toggle.ps1 -Setup"; exit 1; }
[ ! -f "$SETUP_SCRIPT" ] && { echo "Error: setup-wsl.sh not found at $SETUP_SCRIPT"; exit 1; }

echo "Starting CHROTE setup from: $CHROTE_DIR"
export CHROTE_SRC="$CHROTE_DIR"
tr -d '\r' < "$SETUP_SCRIPT" | bash
