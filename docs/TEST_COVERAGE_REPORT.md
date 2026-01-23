# Test Coverage & Strategy Update

## Overview
We have transitioned from a purely manual/E2E testing approach to a comprehensive "Pyramid" strategy including Unit Tests, Component Tests, and Real-Backend Integration Tests.

## New Testing Layers

### 1. Frontend Unit Tests (Vitest)
Located in `dashboard/src/`. These tests run in a simulated browser environment (JSDOM) and are extremely fast (~1-2 seconds).

**Key Areas Covered:**
- **Utilities:** `src/utils/roleDetection.test.ts` - Validates complex logic for parsing role strings.
- **Components:** `src/components/RoleBadge.test.tsx` - Validates UI rendering and class assignment logic.
- **Hooks:** `src/hooks/useKeyboardShortcuts.test.ts` - Validates keybinding logic and event handling.

**How to Run:**
```bash
cd dashboard
npm test
# or for UI mode
npm run test:ui
```

### 2. Real-Backend Integration Tests (Playwright + Go)
Located in `dashboard/tests/integration/`. These tests build the actual Go backend, spin it up against the local system state (real tmux/files), and verify the API contract.

**Key Areas Covered:**
- **API Contract:** Verifies `/api/tmux/sessions` returns the correct JSON structure.
- **Data Integrity:** Confirm actual tmux sessions are visible to the frontend client.

**How to Run:**
```bash
# From project root
./test-integration.sh
```

### 3. End-to-End Tests (Playwright)
Existing tests in `dashboard/tests/`. These continue to serve as the high-level validation.

## Next Steps
1. **Expand Unit Coverage:** Add tests for `SessionContext` and `WebSocket` logic.
2. **CI Pipeline:** Configure GitHub Actions to run `npm test` on PRs.
3. **Mocking Strategy:** Refine backend mocks for scenarios where "Real Backend" is too flaky or dangerous (e.g., file deletion).
