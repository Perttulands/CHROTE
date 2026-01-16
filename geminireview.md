# Gemini Codebase Review

## Overview
Date: January 16, 2026

This review covers the `AgentArena` codebase, focusing on the Dashboard (React), API (Node.js), and internal Go code. The codebase generally shows good separation of concerns, but there are opportunities for performance optimization, reducing redundancy, and improving maintainability.

## 1. Redundant Code & Logic Duplication

### API Layer (`api/`)
- **Session Sorting Logic**: 
  - `api/utils.js` exports a `sortSessions` function.
  - `api/server.js` re-implements this sorting logic inline within the `GET /api/tmux/sessions` handler.
  - **Recommendation**: Refactor `server.js` to import and use `sortSessions` from `utils.js`.

### Dashboard (`dashboard/`)
- **FilesView Component**:
  - `dashboard/src/components/FilesView.tsx` is a re-export facade.
  - The actual implementation resides in `dashboard/src/components/FilesView/index.tsx`.
  - While not strictly redundant, the `index.tsx` file contains multiple component definitions (`FileRow`, `FileGridItem`, `ContextMenu`, `InboxPanel`, `UploadZone`) within a single 600+ line file.
  - **Recommendation**: Extract these sub-components into their own files within the `components/FilesView/components/` directory (some already exist there but might be unused or drift has occurred).

## 2. Long Files & Refactoring Targets

### `dashboard/src/components/FilesView/index.tsx` (~650 lines)
This file is becoming a "God component" for the file browser. It contains:
- `Breadcrumbs`
- `FileRow` / `FileGridItem`
- `ContextMenu`
- `NewFolderDialog` / `DeleteDialog`
- `UploadZone`
- `InboxPanel`
- `InfoPanel`
- Main `FilesView` logic

**Refactoring Plan:**
1. Move `InboxPanel` to `dashboard/src/components/FilesView/components/InboxPanel.tsx`.
2. Move Dialogs to `dashboard/src/components/FilesView/components/Dialogs.tsx`.
3. Move `FileRow` and `FileGridItem` to their own files.
4. Keep `index.tsx` focused on state management and layout.

### `api/server.js` (~300 lines)
This file handles all routes, `tmux` execution logic, input validation, and error handling.
- **Refactoring Plan**: Split into route handlers (e.g., `routes/tmux.js`) and controllers. Move `tmox` specific command execution to a `services/tmuxService.js` to isolate `execSync` calls.

## 3. Performance & Stability Issues

### Node.js Event Loop Blocking
- **Critical Issue**: The API uses `execSync` for all `tmux` interactions.
  - `execSync('tmux list-sessions ...')`
  - `execSync('tmux new-session ...')`
  - `execSync('tmux kill-server ...')`
- **Impact**: If the `tmux` command hangs or is slow (e.g., system load), the entirely API server freezes. No other requests can be processed.
- **Fix**: Switch to `child_process.exec` (callback-based) or `util.promisify(exec)` to make these calls asynchronous.

### Recursive Sync File Operations in Beads Routes
- `api/beads-routes.js` uses recursive synchronous file searching in `/projects` endpoint:
  ```javascript
  const entries = fs.readdirSync(dir, { withFileTypes: true }); // Sync!
  ```
- **Impact**: Searching deep directory trees (like `/code`) synchronously will block the event loop, potentially for seconds.
- **Fix**: Use `fs.promises.readdir` or a library like `glob` or `fast-glob`.

### React Performance
- `files/src/context/SessionContext.tsx`: The `refreshSessions` function runs on an interval. It performs a state update `setWindows` even if little changed, potentially causing meaningful re-renders. The logic to detect changes is decent but complex.
- **Optimization**: Ensure `sessionsCache` in the API is tuned correctly (current TTL 1s is good) to prevent `tmux` process spam.

## 4. Code Quality & Bugs

### Hardcoded Paths
- **InboxPanel**: Uses `const INBOX_PATH = '/code/incoming'`. If the container mount point changes, this breaks.
- **FilesView**: The `fetchDirectory` logic is robust, but the error messages in `dashboard/src/components/FilesView/fileService.ts` for "Network error" might hide actual connection refusal issues if the API server is down.

### Go Migration (`AgentArena_go`)
- The presence of `AgentArena_go` and `internal/tmux` is a positive step.
- **Recommendation**: Prioritize moving the `tmux` interaction logic to Go. Go's `os/exec` is robust, and goroutines handle concurrency naturally, solving the `execSync` blocking issue found in the Node.js version.

## 5. Security Check
- **Arbitrary Command Execution Risk**: 
  - `api/server.js` creates sessions with: `tmux new-session -d -s "${name}"`.
  - Input validation: `if (!/^[a-zA-Z0-9_-]+$/.test(name))` is present.
  - **Verdict**: Validated. The regex prevents command injection via session name.
- **Beads Routes**:
  - `execBvCommand` takes `args`. If `args` comes from user input without validation, it could be dangerous. Currently, it seems internally controlled or strictly typed in usage.
  - **Recommendation**: Verify callers of `execBvCommand` do not pass unsanitized user input as arguments.

## Action Plan
1. **Immediate**: Fix `api/server.js` to use `sortSessions` from `utils.js`.
2. **High Priority**: Refactor `api/server.js` and `api/beads-routes.js` to use **async** filesystem and process calls (`exec` vs `execSync`).
3. **Medium Priority**: Break down `dashboard/src/components/FilesView/index.tsx` into smaller components.
4. **Long Term**: Continue with the Go rewrite for the backend.
