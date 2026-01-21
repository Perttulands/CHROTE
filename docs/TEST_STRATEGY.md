# CHROTE Test Strategy

Comprehensive testing approach for the CHROTE codebase with copy-pasteable commands.

---

## Table of Contents

1. [Test Infrastructure Overview](#test-infrastructure-overview)
2. [Running Tests](#running-tests)
3. [Go Backend Tests](#go-backend-tests)
4. [Frontend E2E Tests (Playwright)](#frontend-e2e-tests-playwright)
5. [Manual Testing Procedures](#manual-testing-procedures)
6. [CI/CD Integration](#cicd-integration)
7. [Writing New Tests](#writing-new-tests)
8. [Test Data and Mocking](#test-data-and-mocking)

---

## Test Infrastructure Overview

CHROTE uses a two-tier testing strategy:

| Layer | Framework | Location | Purpose |
|-------|-----------|----------|---------|
| **Backend** | Go testing | `src/**/*_test.go` | Unit tests, integration tests, API validation |
| **Frontend** | Playwright | `dashboard/tests/*.spec.ts` | E2E browser tests, UI interactions |

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
# Run ALL tests (backend + frontend)
cd /code && cd src && go test ./... && cd ../dashboard && npm test

# Backend only
cd /code/src && go test ./...

# Frontend only
cd /code/dashboard && npm test

# Frontend with UI mode (interactive)
cd /code/dashboard && npm run test:ui

# Frontend headed mode (see browser)
cd /code/dashboard && npm run test:headed
```

### Full Test Suite

```bash
# Complete test run with verbose output
cd /code/src && go test -v ./...
cd /code/dashboard && npm test
```

---

## Go Backend Tests

### Run All Backend Tests

```bash
cd /code/src
go test ./...
```

### Run with Verbose Output

```bash
cd /code/src
go test -v ./...
```

### Run Specific Package

```bash
# API tests only
cd /code/src
go test -v ./internal/api/...

# Core utilities only
cd /code/src
go test -v ./internal/core/...
```

### Run Single Test

```bash
cd /code/src

# Run specific test by name
go test -v -run TestIntegration_FullAPIRouting ./internal/api/

# Run tests matching pattern
go test -v -run "TestHealth" ./internal/api/
```

### Run with Coverage

```bash
cd /code/src

# Coverage report to terminal
go test -cover ./...

# Generate coverage HTML report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Race Detection

```bash
cd /code/src
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

## Frontend E2E Tests (Playwright)

### Prerequisites

```bash
cd /code/dashboard

# Install dependencies (if needed)
npm install

# Install Playwright browsers
npx playwright install
```

### Run All E2E Tests

```bash
cd /code/dashboard
npm test
```

### Interactive UI Mode

```bash
cd /code/dashboard
npm run test:ui
```

Opens Playwright UI for running/debugging tests interactively.

### Headed Mode (Visible Browser)

```bash
cd /code/dashboard
npm run test:headed
```

### Run Specific Test File

```bash
cd /code/dashboard

# Dashboard tests only
npx playwright test dashboard.spec.ts

# File browser tests only
npx playwright test filebrowser.spec.ts

# Settings tests only
npx playwright test settings.spec.ts
```

### Run Specific Test

```bash
cd /code/dashboard

# Run single test by name
npx playwright test -g "should render session panel with groups"

# Run tests matching pattern
npx playwright test -g "drag"
```

### Debug Mode

```bash
cd /code/dashboard

# Debug with Playwright Inspector
npx playwright test --debug

# Debug specific test
npx playwright test -g "drag session" --debug
```

### Generate HTML Report

```bash
cd /code/dashboard

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

1. Open `http://localhost:8080`
2. Verify tabs are visible: Terminal 1, Terminal 2, Files, Beads, Settings
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
curl -X POST http://localhost:8080/api/tmux/sessions \
  -H "Content-Type: application/json" \
  -d '{"name": "test-session"}'

# Test session listing
curl http://localhost:8080/api/tmux/sessions

# Test session deletion
curl -X DELETE http://localhost:8080/api/tmux/sessions/test-session

# Test nuke protection (should fail without header)
curl -X DELETE http://localhost:8080/api/tmux/sessions/all

# Test nuke with confirmation (USE WITH CAUTION)
curl -X DELETE http://localhost:8080/api/tmux/sessions/all \
  -H "X-Nuke-Confirm: yes"
```

### File API Testing

```bash
# List root directories
curl http://localhost:8080/api/files/resources/

# List /code directory
curl http://localhost:8080/api/files/resources/code

# Test path traversal protection (should fail)
curl http://localhost:8080/api/files/resources/code/../../../etc/passwd
```

### Health Check

```bash
# API health
curl http://localhost:8080/api/health

# Expected response: {"status":"ok","timestamp":"..."}
```

### Beads API Testing

```bash
# Check Beads health (bv CLI availability)
curl http://localhost:8080/api/beads/health

# List discovered projects
curl http://localhost:8080/api/beads/projects
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - name: Run Go tests
        run: |
          cd src
          go test -v -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: ./src/coverage.out

  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - name: Install dependencies
        run: |
          cd dashboard
          npm ci
          npx playwright install --with-deps
      - name: Run Playwright tests
        run: |
          cd dashboard
          npm test
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: playwright-report
          path: dashboard/playwright-report/
```

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Run Go tests
cd src
go test ./... || exit 1

# Run Playwright tests (optional, slower)
# cd ../dashboard
# npm test || exit 1
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

# Create Gastown rig sessions
for i in {1..3}; do
  tmux new-session -d -s "gt-gastown-worker$i"
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
rm -f /code/src/coverage.out
rm -f /code/src/coverage.html

# Remove Playwright artifacts
rm -rf /code/dashboard/playwright-report
rm -rf /code/dashboard/test-results

echo "Test artifacts cleaned"
```

---

## Quick Reference

| Task | Command |
|------|---------|
| All backend tests | `cd /code/src && go test ./...` |
| All frontend tests | `cd /code/dashboard && npm test` |
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
cd /code/src
go mod verify

# Clean and retry
go clean -testcache
go test ./...
```

### Playwright Tests Fail

```bash
# Reinstall browsers
cd /code/dashboard
npx playwright install

# Check if dev server starts
npm run dev
# Should start on localhost:5173

# Run with trace for debugging
npx playwright test --trace on
```

### Services Not Available

```bash
# Ensure services are running
systemctl status chrote-server chrote-ttyd

# Check ports
ss -tlnp | grep -E "8080|5173"

# Restart services
sudo systemctl restart chrote-server chrote-ttyd
```
