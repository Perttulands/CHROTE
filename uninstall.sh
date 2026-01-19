#!/bin/bash
# CHROTE Uninstaller
set -e

INSTALL_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.chrote"
SERVICE_DIR="$HOME/.config/systemd/user"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${CYAN}[CHROTE]${NC} $1"; }
success() { echo -e "${GREEN}[CHROTE]${NC} $1"; }
warn() { echo -e "${YELLOW}[CHROTE]${NC} $1"; }

echo ""
echo -e "${CYAN}================================${NC}"
echo -e "${CYAN}   CHROTE Uninstaller${NC}"
echo -e "${CYAN}================================${NC}"
echo ""

# Confirm
read -p "This will remove CHROTE. Continue? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

# Stop services
log "Stopping services..."
systemctl --user stop chrote chrote-ttyd 2>/dev/null || true
systemctl --user disable chrote chrote-ttyd 2>/dev/null || true

# Remove service files
log "Removing service files..."
rm -f "$SERVICE_DIR/chrote.service"
rm -f "$SERVICE_DIR/chrote-ttyd.service"
systemctl --user daemon-reload 2>/dev/null || true

# Remove binaries
log "Removing binaries..."
rm -f "$INSTALL_DIR/chrote-server"
rm -f "$INSTALL_DIR/ttyd"

# Remove config
log "Removing configuration..."
rm -rf "$CONFIG_DIR"

echo ""
success "CHROTE has been uninstalled."
echo ""
warn "Note: Your workspace directory was NOT removed."
warn "Remove it manually if no longer needed."
echo ""
