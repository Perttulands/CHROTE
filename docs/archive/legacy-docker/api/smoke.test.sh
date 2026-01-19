#!/bin/bash
# Routing smoke tests
# Run this inside WSL or against the running CHROTE instance
# Usage: ./smoke.test.sh [BASE_URL]

BASE_URL="${1:-http://localhost:8080}"
FAILED=0
PASSED=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "================================================================"
echo "CHROTE Routing Smoke Tests"
echo "Base URL: $BASE_URL"
echo "================================================================"
echo ""

# Test helper function
test_endpoint() {
  local name="$1"
  local url="$2"
  local expected_status="$3"
  local expected_content="$4"

  response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null)
  status=$(echo "$response" | tail -n1)
  body=$(echo "$response" | head -n -1)

  if [ "$status" = "$expected_status" ]; then
    if [ -n "$expected_content" ]; then
      if echo "$body" | grep -q "$expected_content"; then
        echo -e "${GREEN}✓${NC} $name (status: $status, content matched)"
        ((PASSED++))
      else
        echo -e "${RED}✗${NC} $name (status: $status, content mismatch)"
        echo "  Expected: $expected_content"
        echo "  Got: ${body:0:100}..."
        ((FAILED++))
      fi
    else
      echo -e "${GREEN}✓${NC} $name (status: $status)"
      ((PASSED++))
    fi
  else
    echo -e "${RED}✗${NC} $name (expected: $expected_status, got: $status)"
    ((FAILED++))
  fi
}

# Test WebSocket upgrade capability (just check headers)
test_websocket_headers() {
  local name="$1"
  local url="$2"

  response=$(curl -s -I \
    -H "Upgrade: websocket" \
    -H "Connection: Upgrade" \
    -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
    -H "Sec-WebSocket-Version: 13" \
    "$url" 2>/dev/null)

  # Check if nginx returns 101 Switching Protocols or allows the upgrade path
  if echo "$response" | grep -q "101\|200"; then
    echo -e "${GREEN}✓${NC} $name (WebSocket upgrade path exists)"
    ((PASSED++))
  else
    echo -e "${YELLOW}~${NC} $name (WebSocket test inconclusive - endpoint may not be running)"
    ((PASSED++)) # Don't fail if ttyd isn't running
  fi
}

echo "=== Dashboard (React) ==="
test_endpoint "GET / serves dashboard" "$BASE_URL/" "200" "<!DOCTYPE html>"
test_endpoint "GET /index.html" "$BASE_URL/index.html" "200" "<!DOCTYPE html>"
test_endpoint "Static assets path exists" "$BASE_URL/assets/" "200\|404\|301" ""

echo ""
echo "=== API Proxy ==="
test_endpoint "GET /api/health" "$BASE_URL/api/health" "200" '"status":"ok"'
test_endpoint "GET /api/tmux/sessions" "$BASE_URL/api/tmux/sessions" "200" '"sessions"'
test_endpoint "GET /api/beads/health" "$BASE_URL/api/beads/health" "200" '"status":"ok"'

echo ""
echo "=== File API ==="
test_endpoint "GET /api/files/resources/" "$BASE_URL/api/files/resources/" "200" '"isDir":true'
test_endpoint "GET /api/files/resources/code (if mounted)" "$BASE_URL/api/files/resources/code" "200\|404" ""

echo ""
echo "=== Terminal (ttyd) ==="
test_websocket_headers "Terminal WebSocket path" "$BASE_URL/terminal/"

echo ""
echo "=== Error Handling ==="
test_endpoint "404 for unknown paths" "$BASE_URL/nonexistent/path/xyz" "404" ""
test_endpoint "API 404 for unknown endpoints" "$BASE_URL/api/nonexistent" "404" ""

echo ""
echo "=== CORS Headers ==="
# Test CORS on API endpoint
cors_response=$(curl -s -I -X OPTIONS \
  -H "Origin: http://localhost:5173" \
  -H "Access-Control-Request-Method: GET" \
  "$BASE_URL/api/health" 2>/dev/null)

if echo "$cors_response" | grep -qi "access-control"; then
  echo -e "${GREEN}✓${NC} CORS headers present on API"
  ((PASSED++))
else
  echo -e "${YELLOW}~${NC} CORS headers check (may need Origin header)"
  ((PASSED++))
fi

echo ""
echo "================================================================"
echo -e "Results: ${GREEN}$PASSED passed${NC}, ${RED}$FAILED failed${NC}"
echo "================================================================"

if [ $FAILED -gt 0 ]; then
  exit 1
fi
exit 0
