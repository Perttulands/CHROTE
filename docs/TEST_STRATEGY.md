# CHROTE Test Strategy

Comprehensive testing approach for the CHROTE codebase with copy-pasteable commands.

---

## Table of Contents

1. [Test Infrastructure Overview](#test-infrastructure-overview)
2. [Running Tests](#running-tests)
3. [Go Backend Tests](#go-backend-tests)
4. [Frontend Tests](#frontend-tests)
5. [Manual Testing Procedures](#manual-testing-procedures)
6. [CI/CD Integration](#cicd-integration)
7. [Writing New Tests](#writing-new-tests)
8. [Test Data and Mocking](#test-data-and-mocking)

---

## Test Infrastructure Overview

CHROTE uses this testing strategy:

| Layer | Framework | Location | Purpose |
|-------|-----------|----------|---------|
| **Backend** | Go testing | `src/**/*_test.go` | Unit tests, integration tests, API validation |
| **Frontend Unit** | Vitest | `dashboard/src/**/*.test.ts(x)` | Component, hook, service, and utility tests |
| **Frontend E2E** | Playwright | `dashboard/tests/*.spec.ts` | Mocked deterministic browser tests |
| **Live Frontend Integration** | Playwright | `dashboard/tests/integration/*.spec.ts` | Explicit CHROTE backend/terminal smoke tests |

### Directory Structure

```
CHROTE/
├── src/
│   └── internal/
│       ├── api/
│       │   ├── files_test.go      # Files API tests
│       │   ├── health_test.go     # Health endpoint tests
│       │   ├── tmux_test.go       # Tmux API tests
│       │   └── integration_test.go # API integration tests
│       └── core/
│           ├── pathutil_test.go   # Path utility tests
│           ├── response_test.go   # Response helper tests
│           └── session_test.go    # Session logic tests
└── dashboard/
    ├── tests/
    │   ├── dashboard.spec.ts      # Main dashboard E2E tests
    │   ├── filebrowser.spec.ts    # File browser E2E tests
    │   ├── beads.spec.ts          # Beads integration tests
    │   ├── settings.spec.ts       # Settings page tests
    │   ├── path-mapping.spec.ts   # Path handling tests
    │   └── mock-api.ts            # API mocking utilities
    └── playwright.config.ts       # Playwright configuration
```

---

## Running Tests

### Quick Commands

```bash
# Stable local quality gate
cd /path/to/chrote
python3 -m py_compile scripts/doc-lint.py
scripts/doc-lint.py

cd /path/to/chrote/src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go vet ./...
go test ./...
go test -race ./...
go test -cover ./...

cd /path/to/chrote/dashboard
npm ci
npm run lint
npm run build
npm run test:unit -- --coverage
npm audit --audit-level=moderate
npm test

# Backend only
cd /path/to/chrote/src && go test ./...

# Frontend unit tests
cd /path/to/chrote/dashboard && npm run test:unit

# Frontend lint
cd /path/to/chrote/dashboard && npm run lint

# Frontend unit coverage
cd /path/to/chrote/dashboard && npm run test:unit -- --coverage

# Frontend mocked browser tests
cd /path/to/chrote/dashboard && npm test

# Live backend/terminal browser tests
cd /path/to/chrote/dashboard && CHROTE_TEST_URL=http://127.0.0.1:8095 npm run test:live

# Frontend with UI mode (interactive)
cd /path/to/chrote/dashboard && npm run test:ui

# Frontend headed mode (see browser)
cd /path/to/chrote/dashboard && npm run test:headed
```

### Full Test Suite

```bash
# Complete test run with verbose output
cd /path/to/chrote/src && go test -v ./...
cd /path/to/chrote/dashboard && npm run test:unit && npm test
```

---

## Go Backend Tests

### Run All Backend Tests

```bash
cd /path/to/chrote/src
go test ./...
```

### Run with Verbose Output

```bash
cd /path/to/chrote/src
go test -v ./...
```

### Run Specific Package

```bash
# API tests only
cd /path/to/chrote/src
go test -v ./internal/api/...

# Core utilities only
cd /path/to/chrote/src
go test -v ./internal/core/...
```

### Run Single Test

```bash
cd /path/to/chrote/src

# Run specific test by name
go test -v -run TestIntegration_FullAPIRouting ./internal/api/

# Run tests matching pattern
go test -v -run "TestHealth" ./internal/api/
```

### Run with Coverage

```bash
cd /path/to/chrote/src

# Coverage report to terminal
go test -cover ./...

# Generate coverage HTML report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Race Detection

```bash
cd /path/to/chrote/src
go test -race ./...
```

### Test Categories

| File | Tests | Purpose |
|------|-------|---------|
| `integration_test.go` | API routing, response format, error handling | Full API integration |
| `health_test.go` | Health endpoint | Service availability |
| `tmux_test.go` | Session CRUD, naming validation | Tmux API |
| `files_test.go` | File listing, path traversal protection | Files API |
| `pathutil_test.go` | Path validation, normalization | Security utilities |
| `session_test.go` | Session grouping, sorting | Business logic |
| `response_test.go` | JSON response helpers | API utilities |

---

## Frontend Tests

### Prerequisites

```bash
cd /path/to/chrote/dashboard

# Install dependencies (if needed)
npm ci

# Install Playwright browsers
npx playwright install chromium
```

### Unit And Coverage

```bash
cd /path/to/chrote/dashboard
npm run test:unit
npm run test:unit -- --coverage
```

Coverage output is written to `dashboard/coverage/`.

### Run Default Playwright Gate

```bash
cd /path/to/chrote/dashboard
npm test
```

The default Playwright suite is the mocked deterministic browser gate. It excludes `dashboard/tests/integration/**`.

When Playwright starts the local Vite dev server, it sets `CHROTE_PLAYWRIGHT_MOCKED=1`; `dashboard/vite.config.ts` disables backend proxying in that mode. Default browser tests must mock every `/api/**` and terminal request.

### Run Live Backend Integration Tests

```bash
cd /path/to/chrote/dashboard
CHROTE_TEST_URL=http://127.0.0.1:8095 npm run test:live
```

Live integration specs require a running CHROTE backend and terminal proxy. They are intentionally excluded from `npm test` and CI's default Playwright gate.

Live smokes are operator-run only in this workspace because GitHub-hosted runners do not have access to the local CHROTE runtime, tmux socket, or terminal proxy. Use `CHROTE_TEST_URL` to point at the operator-approved CHROTE backend under test. The current `/srv` proving lane is `/srv/chrote` with data under `/srv/data/chrote`, `chrote-srv.service`, HTTP `8095`, and ttyd `7686`; set `CHROTE_TEST_URL=http://127.0.0.1:8094` only for the legacy rollback lane. Do not hard-code private hostnames or tailnet addresses into the test suite.

### Interactive UI Mode

```bash
cd /path/to/chrote/dashboard
npm run test:ui
```

Opens Playwright UI for running/debugging tests interactively.

### Headed Mode (Visible Browser)

```bash
cd /path/to/chrote/dashboard
npm run test:headed
```

### Run Specific Test File

```bash
cd /path/to/chrote/dashboard

# Dashboard tests only
npx playwright test dashboard.spec.ts

# File browser tests only
npx playwright test filebrowser.spec.ts

# Settings tests only
npx playwright test settings.spec.ts
```

### Run Specific Test

```bash
cd /path/to/chrote/dashboard

# Run single test by name
npx playwright test -g "should render session panel with groups"

# Run tests matching pattern
npx playwright test -g "drag"
```

### Debug Mode

```bash
cd /path/to/chrote/dashboard

# Debug with Playwright Inspector
npx playwright test --debug

# Debug specific test
npx playwright test -g "drag session" --debug
```

### Generate HTML Report

```bash
cd /path/to/chrote/dashboard

# Run tests and generate report
npx playwright test

# Open report
npx playwright show-report
```

### Test Categories

| File | Tests | Coverage |
|------|-------|----------|
| `dashboard.spec.ts` | Session panel, terminal area, drag-drop, keyboard nav | Main dashboard UI |
| `filebrowser.spec.ts` | File listing, navigation, upload | File browser |
| `beads.spec.ts` | Beads integration, project discovery | Issue tracking |
| `settings.spec.ts` | Theme, font size, tmux colors | Settings page |
| `path-mapping.spec.ts` | Windows/WSL path conversion | Path handling |

---

## Manual Testing Procedures

### Dashboard Smoke Test

1. Open the operator-approved backend, usually `http://127.0.0.1:8095` for the `/srv` proving lane.
2. Verify tabs are visible: Terminal, Terminal 2, Files, Agents, Beads, Services, Settings
3. Check session panel shows groups
4. Create a new session
5. Drag session to terminal window
6. Verify terminal loads

```bash
# Create test sessions for smoke testing
tmux new-session -d -s test-shell
tmux new-session -d -s hq-test
tmux new-session -d -s gt-test-1
```

### Terminal Functionality

```bash
# Test session creation via API
CHROTE_URL=http://127.0.0.1:8095      # /srv proving lane
# CHROTE_URL=http://127.0.0.1:8094    # legacy rollback lane

curl -X POST "$CHROTE_URL/api/tmux/sessions" \
  -H "Content-Type: application/json" \
  -d '{"name": "test-session"}'

# Test session listing
curl "$CHROTE_URL/api/tmux/sessions"

# Test session deletion
curl -X DELETE "$CHROTE_URL/api/tmux/sessions/test-session"

# Test nuke protection (should fail without header)
curl -X DELETE "$CHROTE_URL/api/tmux/sessions/all"

# Test nuke with confirmation (USE WITH CAUTION)
curl -X DELETE "$CHROTE_URL/api/tmux/sessions/all" \
  -H "X-Nuke-Confirm: yes"
```

### File API Testing

```bash
# List root directories
curl "$CHROTE_URL/api/files/resources/"

# List /code directory
curl "$CHROTE_URL/api/files/resources/code"

# Test path traversal protection (should fail)
curl "$CHROTE_URL/api/files/resources/code/../../../etc/passwd"
```

### Health Check

```bash
# API health
curl "$CHROTE_URL/api/health"

# Expected response: {"status":"ok","timestamp":"..."}
```

### Beads API Testing

```bash
# Check Beads health (bv CLI availability)
curl "$CHROTE_URL/api/beads/health"

# List discovered projects
curl "$CHROTE_URL/api/beads/projects"
```

---

## CI/CD Integration

The CI workflow lives at `.github/workflows/ci.yml`. It runs on pull requests and pushes to `main`/`master` with Go 1.23 and Node 20:

```bash
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go vet ./...
go test ./...
go test -race ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

cd dashboard
npm ci
npm run lint
npm run build
npm run test:unit -- --coverage
npm audit --audit-level=moderate
npx playwright install --with-deps chromium
npm test

rm -rf src/internal/dashboard/dist
cp -r dashboard/dist src/internal/dashboard/
cd src
go test ./...
go build -o /tmp/chrote-server-ci ./cmd/server
```

The release workflow also uses Go 1.23 and Node 20. It runs the same Go format/vet/test/race assumptions, dashboard lint/build/unit-coverage/audit, copies the fresh dashboard `dist` into `src/internal/dashboard/dist`, and performs an embedded-dashboard Go build smoke before producing tag artifacts.

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Run Go checks
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))" || exit 1
go vet ./... || exit 1
go test ./... || exit 1
go test -race ./... || exit 1
go test -cover ./... || exit 1

# Run dashboard stable gates
cd ../dashboard
npm run lint || exit 1
npm run build || exit 1
npm run test:unit -- --coverage || exit 1
npm audit --audit-level=moderate || exit 1
npm test || exit 1
```

---

## Writing New Tests

### Go Backend Test Template

```go
package api

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestMyFeature(t *testing.T) {
    // Setup
    mux := http.NewServeMux()
    handler := NewMyHandler()
    handler.RegisterRoutes(mux)

    // Test cases
    t.Run("happy path", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodGet, "/api/my/endpoint", nil)
        rec := httptest.NewRecorder()
        mux.ServeHTTP(rec, req)

        if rec.Code != http.StatusOK {
            t.Errorf("Expected 200, got %d", rec.Code)
        }

        var response map[string]interface{}
        if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
            t.Errorf("Invalid JSON: %v", err)
        }
    })

    t.Run("error case", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodGet, "/api/my/endpoint/invalid", nil)
        rec := httptest.NewRecorder()
        mux.ServeHTTP(rec, req)

        if rec.Code != http.StatusBadRequest {
            t.Errorf("Expected 400, got %d", rec.Code)
        }
    })
}
```

### Playwright Test Template

```typescript
import { test, expect } from '@playwright/test'
import { mockApiRoutes } from './mock-api'

test.describe('My Feature', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('should do something', async ({ page }) => {
    // Interact with the page
    await page.click('.my-button')

    // Assert
    await expect(page.locator('.result')).toBeVisible()
    await expect(page.locator('.result')).toContainText('Expected text')
  })

  test('should handle error state', async ({ page }) => {
    // Mock error response
    await page.route('**/api/my/endpoint', route => {
      route.fulfill({
        status: 500,
        body: JSON.stringify({ error: 'Server error' })
      })
    })

    await page.click('.trigger-action')
    await expect(page.locator('.error-message')).toBeVisible()
  })
})
```

---

## Test Data and Mocking

### Mock API (Playwright)

The `mock-api.ts` file provides API mocking for E2E tests:

```typescript
// dashboard/tests/mock-api.ts
import { Page } from '@playwright/test'

export async function mockApiRoutes(page: Page) {
  // Mock tmux sessions
  await page.route('**/api/tmux/sessions', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        sessions: [
          { name: 'hq-mayor', attached: false },
          { name: 'hq-deacon', attached: true },
          { name: 'gt-gastown-jack', attached: false },
          // ... more mock sessions
        ],
        groups: [
          { name: 'HQ', priority: 0, sessions: ['hq-mayor', 'hq-deacon'] },
          // ... more groups
        ],
        timestamp: new Date().toISOString()
      })
    })
  })

  // Mock health endpoint
  await page.route('**/api/health', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok', timestamp: new Date().toISOString() })
    })
  })

  // Add more mocks as needed...
}
```

### Test Session Script

```bash
#!/bin/bash
# test-sessions.sh - Create test sessions for manual testing

# Clean existing test sessions
tmux list-sessions -F "#{session_name}" | grep "^test-" | xargs -I {} tmux kill-session -t {}

# Create HQ sessions
tmux new-session -d -s hq-mayor
tmux new-session -d -s hq-deacon

# Create grouped agent-style sessions
for i in {1..3}; do
  tmux new-session -d -s "agent-worker$i"
done

# Create misc sessions
tmux new-session -d -s shell
tmux new-session -d -s test-misc

echo "Test sessions created:"
tmux list-sessions
```

### Cleanup Script

```bash
#!/bin/bash
# cleanup-tests.sh - Remove test artifacts

# Remove test sessions
tmux list-sessions -F "#{session_name}" | grep -E "^(test-|gt-test)" | xargs -I {} tmux kill-session -t {}

# Remove test coverage files
rm -f /path/to/chrote/src/coverage.out
rm -f /path/to/chrote/src/coverage.html

# Remove Playwright artifacts
rm -rf /path/to/chrote/dashboard/playwright-report
rm -rf /path/to/chrote/dashboard/test-results

echo "Test artifacts cleaned"
```

---

## Quick Reference

| Task | Command |
|------|---------|
| All backend tests | `cd /path/to/chrote/src && go test ./...` |
| Backend format check | `cd /path/to/chrote/src && test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"` |
| Backend static check | `cd /path/to/chrote/src && go vet ./...` |
| Backend race tests | `cd /path/to/chrote/src && go test -race ./...` |
| Frontend unit tests | `cd /path/to/chrote/dashboard && npm run test:unit` |
| Frontend lint | `cd /path/to/chrote/dashboard && npm run lint` |
| Frontend coverage | `cd /path/to/chrote/dashboard && npm run test:unit -- --coverage` |
| Frontend dependency audit | `cd /path/to/chrote/dashboard && npm audit --audit-level=moderate` |
| Frontend mocked browser tests | `cd /path/to/chrote/dashboard && npm test` |
| Live backend browser tests | `cd /path/to/chrote/dashboard && CHROTE_TEST_URL=http://127.0.0.1:8095 npm run test:live` |
| Backend verbose | `go test -v ./...` |
| Backend coverage | `go test -cover ./...` |
| Frontend interactive | `npm run test:ui` |
| Frontend headed | `npm run test:headed` |
| Single Go test | `go test -v -run TestName ./...` |
| Single Playwright test | `npx playwright test -g "test name"` |
| Debug Playwright | `npx playwright test --debug` |
| View report | `npx playwright show-report` |

---

## Troubleshooting Tests

### Go Tests Fail

```bash
# Check Go version
go version
# Should be 1.23+

# Verify dependencies
cd /path/to/chrote/src
go mod verify

# Clean and retry
go clean -testcache
go test ./...
```

### Playwright Tests Fail

```bash
# Reinstall browsers
cd /path/to/chrote/dashboard
npx playwright install

# Check if dev server starts
npm run dev
# Should start on localhost:5173

# Run with trace for debugging
npx playwright test --trace on
```

### Services Not Available

```bash
# Ensure the intended service is running.
systemctl status chrote-srv.service --no-pager       # /srv proving lane
systemctl --user status chrote.service               # legacy rollback lane

# Check ports
ss -tlnp | grep -E "8095|7686|5173"

# Restart services
systemctl restart chrote-srv.service
```
