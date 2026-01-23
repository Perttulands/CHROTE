#!/bin/bash
set -e

# Project Root
ROOT_DIR="$(dirname "$(readlink -f "$0")")"
cd "$ROOT_DIR"

echo "=== Setup: Building Backend ==="
if ! command -v go &> /dev/null; then
    echo "Error: 'go' is not installed."
    exit 1
fi

# Build binary
cd src
go build -o ../chrote-server-integ ./cmd/server/main.go
cd ..
echo "Backend built: ./chrote-server-integ"

echo "=== Setup: Starting Backend ==="
# Start in background
# Using a different ttyd port (7682) to avoid conflict if ttyd is running elsewhere, 
# though we are mostly testing the API on 8080.
./chrote-server-integ --port 8080 --ttyd-port 7682 &
SERVER_PID=$!

echo "Backend started with PID $SERVER_PID. Waiting for readiness..."

cleanup() {
  echo "=== Teardown: Stopping Backend ==="
  if ps -p $SERVER_PID > /dev/null; then
    kill $SERVER_PID
    echo "Backend stopped."
  fi
  rm -f chrote-server-integ
}
trap cleanup EXIT

# Poll for health
MAX_RETRIES=10
RETRIES=0
until curl -s http://localhost:8080/api/health > /dev/null; do
  RETRIES=$((RETRIES+1))
  if [ $RETRIES -ge $MAX_RETRIES ]; then
    echo "Error: Backend failed to become ready after 10 seconds."
    exit 1
  fi
  sleep 1
  echo -n "."
done
echo "Backend is ready!"

echo "=== Test: Running Integration Specs ==="
cd dashboard
npx playwright test tests/integration/real-backend.spec.ts

echo "=== Success: Integration Tests Passed ==="
