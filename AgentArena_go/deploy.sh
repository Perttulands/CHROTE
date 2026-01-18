#!/bin/bash
# AgentArena Go - Deployment Script
# Deploys the arena-server binary to WSL

set -e

BINARY="arena-server"
INSTALL_PATH="/usr/local/bin/arena-server"
SERVICE_NAME="arena"

echo "=== AgentArena Go Deployment ==="

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    echo "Error: Binary '$BINARY' not found. Build it first with:"
    echo "  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o arena-server ./cmd/server"
    exit 1
fi

# Make executable
chmod +x "$BINARY"

echo "1. Copying binary to $INSTALL_PATH..."
sudo cp "$BINARY" "$INSTALL_PATH"
sudo chmod +x "$INSTALL_PATH"

echo "2. Creating systemd service..."
sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null << 'EOF'
[Unit]
Description=AgentArena Go Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/arena-server --port 8080 --ttyd-port 7681
Restart=always
RestartSec=5
Environment=TMUX_TMPDIR=/tmp

[Install]
WantedBy=multi-user.target
EOF

echo "3. Reloading systemd..."
sudo systemctl daemon-reload

echo "4. Enabling and starting service..."
sudo systemctl enable "$SERVICE_NAME"
sudo systemctl restart "$SERVICE_NAME"

echo "5. Checking service status..."
sleep 2
sudo systemctl status "$SERVICE_NAME" --no-pager || true

echo ""
echo "=== Deployment Complete ==="
echo "Dashboard: http://localhost:8080/"
echo "API:       http://localhost:8080/api/health"
echo "Terminal:  http://localhost:8080/terminal/"
echo ""
echo "Commands:"
echo "  sudo systemctl status arena    # Check status"
echo "  sudo systemctl restart arena   # Restart"
echo "  sudo journalctl -u arena -f    # View logs"
