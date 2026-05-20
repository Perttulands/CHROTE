# CHROTE: Markdown Viewer Feature

## Problem Statement

When using CHROTE to monitor Claude Code sessions, users frequently see markdown file paths in terminal output—particularly plan files at `~/.claude/plans/*.md`. Currently, viewing these files requires:

1. Note the file path from terminal output
2. Switch to the Files tab
3. Navigate through directories to find the file
4. Download the file
5. Open in an external editor/viewer

This 5-step workflow breaks focus and slows down plan review during active agent supervision.

---

## Solution

A keyboard-triggered markdown popup viewer. Users select a file path in the terminal, press `Ctrl+Shift+M`, and instantly see the rendered markdown in a draggable overlay—similar to Claude's artifacts.

---

## User Journey

```
1. User monitors Claude Code session in terminal pane
                    ↓
2. Claude outputs: "Writing plan to ~/.claude/plans/feature-x.md"
                    ↓
3. User selects the path text with mouse
                    ↓
4. User presses Ctrl+Shift+M
                    ↓
5. Popup appears with rendered markdown:
   ┌─────────────────────────────────────┐
   │ 📄 feature-x.md            [↻] [✕] │
   ├─────────────────────────────────────┤
   │ # Feature X Implementation Plan    │
   │                                     │
   │ ## Overview                         │
   │ This plan covers...                 │
   │                                     │
   │ ## Steps                            │
   │ 1. First we need to...              │
   │                                     │
   │ ```typescript                       │
   │ const foo = "bar";                  │
   │ ```                                 │
   └─────────────────────────────────────┘
                    ↓
6. User reviews plan, closes popup, continues monitoring
```

---

## Design

### Architecture

```
Terminal iframe → User selects text → Ctrl+Shift+M
        ↓
    extractMarkdownPath(selection)
        ↓
    resolveClaudePath(path)  →  null? → show toast "No valid path"
        ↓
    GET /api/files/claude-content/{path}
        ↓
    403/404/500? → show error in popup
        ↓
    Render markdown in FloatingModal
```

### Security Boundaries

**Problem**: Adding `/root` or `/home/arena` to the existing `ALLOWED_ROOTS` would expose sensitive files (SSH keys, API tokens, configs) to all file operations (read/write/delete).

**Solution**: Scoped read-only endpoint

| Endpoint | Access | Operations |
|----------|--------|------------|
| `/api/files/resources/*` | `/code`, `/vault` | read, write, delete, rename |
| `/api/files/claude-content/*` | `~/.claude/` only | read only |

The new endpoint:
- Only allows paths within `~/.claude/` (both `/root/.claude` and `/home/arena/.claude` for Docker/WSL compatibility)
- Read-only (no write/delete/rename)
- Uses `path.relative()` for proper traversal prevention
- 1MB file size limit

### Components

**Backend** (Go):
- `resolveClaudePath()` - validates path is within `~/.claude/`, prevents traversal
- `GET /api/files/claude-content/*` - returns file content as UTF-8 text

**Frontend** (React):
- `extractMarkdownPath(text)` - regex extraction of `.md` paths from selection
- `resolveClaudePath(path)` - client-side path normalization (server is source of truth)
- `MarkdownContent` - renders markdown with syntax highlighting, sanitizes `javascript:` links
- Extended `FloatingModal` - reuses existing draggable modal, adds markdown mode

---

## Test-Driven Development Plan

### Phase 1: Backend Security Tests

Write tests first, then implement `resolveClaudePath()` and the endpoint.

```go
// file: api/files_test.go

func TestResolveClaudePath(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
        errMsg  string
    }{
        // Valid paths
        {"root claude plans", "/root/.claude/plans/test.md", false, ""},
        {"arena claude plans", "/home/arena/.claude/plans/test.md", false, ""},
        {"nested path", "/root/.claude/projects/foo/bar.md", false, ""},

        // Invalid: outside .claude
        {"root ssh", "/root/.ssh/id_rsa", true, "Path must be within ~/.claude/"},
        {"arena ssh", "/home/arena/.ssh/id_rsa", true, "Path must be within ~/.claude/"},
        {"etc passwd", "/etc/passwd", true, "Path must be within ~/.claude/"},
        {"code directory", "/code/project/file.md", true, "Path must be within ~/.claude/"},

        // Invalid: traversal attempts
        {"traversal up", "/root/.claude/../.ssh/id_rsa", true, "Path traversal detected"},
        {"traversal encoded", "/root/.claude/%2e%2e/.ssh/id_rsa", true, "Path traversal detected"},
        {"double slash", "/root/.claude//../../etc/passwd", true, "Path traversal detected"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := resolveClaudePath(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
                assert.NotEmpty(t, result)
            }
        })
    }
}

func TestClaudeContentEndpoint(t *testing.T) {
    // Setup test server
    srv := setupTestServer(t)

    t.Run("returns 403 for path outside .claude", func(t *testing.T) {
        resp := srv.Get("/api/files/claude-content/root/.ssh/id_rsa")
        assert.Equal(t, 403, resp.StatusCode)
        assert.JSONEq(t, `{"error":"Path must be within ~/.claude/"}`, resp.Body)
    })

    t.Run("returns 403 for traversal attempt", func(t *testing.T) {
        resp := srv.Get("/api/files/claude-content/root/.claude/../.ssh/id_rsa")
        assert.Equal(t, 403, resp.StatusCode)
    })

    t.Run("returns 404 for nonexistent file", func(t *testing.T) {
        resp := srv.Get("/api/files/claude-content/root/.claude/plans/nonexistent.md")
        assert.Equal(t, 404, resp.StatusCode)
        assert.JSONEq(t, `{"error":"File not found"}`, resp.Body)
    })

    t.Run("returns 400 for directory", func(t *testing.T) {
        // Create test directory
        os.MkdirAll("/tmp/test/.claude/plans", 0755)
        resp := srv.Get("/api/files/claude-content/tmp/test/.claude/plans")
        assert.Equal(t, 400, resp.StatusCode)
    })

    t.Run("returns 413 for file over 1MB", func(t *testing.T) {
        // Create 2MB file
        createTestFile(t, "/tmp/test/.claude/large.md", 2*1024*1024)
        resp := srv.Get("/api/files/claude-content/tmp/test/.claude/large.md")
        assert.Equal(t, 413, resp.StatusCode)
    })

    t.Run("returns content for valid file", func(t *testing.T) {
        content := "# Test Plan\n\nThis is a test."
        createTestFile(t, "/tmp/test/.claude/plans/test.md", content)
        resp := srv.Get("/api/files/claude-content/tmp/test/.claude/plans/test.md")
        assert.Equal(t, 200, resp.StatusCode)
        assert.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
        assert.Equal(t, content, resp.Body)
    })
}
```

### Phase 2: Frontend Path Resolution Tests

```typescript
// file: dashboard/src/utils/pathResolver.test.ts

import { extractMarkdownPath, resolveClaudePath } from './pathResolver';

describe('extractMarkdownPath', () => {
  it('extracts ~/.claude paths', () => {
    expect(extractMarkdownPath('Writing to ~/.claude/plans/foo.md'))
      .toBe('~/.claude/plans/foo.md');
  });

  it('extracts absolute /root/.claude paths', () => {
    expect(extractMarkdownPath('File: /root/.claude/plans/bar.md'))
      .toBe('/root/.claude/plans/bar.md');
  });

  it('extracts /home/arena/.claude paths', () => {
    expect(extractMarkdownPath('Saved /home/arena/.claude/test.md'))
      .toBe('/home/arena/.claude/test.md');
  });

  it('returns null for non-.claude paths', () => {
    expect(extractMarkdownPath('/code/project/readme.md')).toBeNull();
    expect(extractMarkdownPath('~/documents/notes.md')).toBeNull();
  });

  it('returns null for non-.md files', () => {
    expect(extractMarkdownPath('~/.claude/config.json')).toBeNull();
  });
});

describe('resolveClaudePath', () => {
  it('expands ~ to /home/arena', () => {
    expect(resolveClaudePath('~/.claude/plans/foo.md'))
      .toBe('/home/arena/.claude/plans/foo.md');
  });

  it('normalizes .. segments', () => {
    expect(resolveClaudePath('/root/.claude/plans/../other/file.md'))
      .toBe('/root/.claude/other/file.md');
  });

  it('returns null for paths outside .claude', () => {
    expect(resolveClaudePath('/root/.ssh/id_rsa')).toBeNull();
    expect(resolveClaudePath('~/.ssh/id_rsa')).toBeNull();
  });

  it('returns null for traversal escaping .claude', () => {
    expect(resolveClaudePath('/root/.claude/../../etc/passwd')).toBeNull();
  });
});
```

### Phase 3: Markdown Rendering Tests

```typescript
// file: dashboard/src/components/MarkdownContent.test.tsx

import { render, screen } from '@testing-library/react';
import { MarkdownContent } from './MarkdownContent';

describe('MarkdownContent', () => {
  it('renders headings', () => {
    render(<MarkdownContent content="# Hello World" />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Hello World');
  });

  it('renders code blocks with syntax highlighting', () => {
    render(<MarkdownContent content="```typescript\nconst x = 1;\n```" />);
    expect(screen.getByText('const')).toBeInTheDocument();
  });

  it('renders tables', () => {
    const table = '| A | B |\n|---|---|\n| 1 | 2 |';
    render(<MarkdownContent content={table} />);
    expect(screen.getByRole('table')).toBeInTheDocument();
  });

  it('blocks javascript: links', () => {
    render(<MarkdownContent content="[click](javascript:alert(1))" />);
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', '#blocked');
  });

  it('opens external links in new tab', () => {
    render(<MarkdownContent content="[example](https://example.com)" />);
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });
});
```

### Phase 4: Integration Tests

```typescript
// file: dashboard/src/components/FloatingModal.test.tsx

describe('FloatingModal markdown mode', () => {
  it('fetches and displays markdown content', async () => {
    // Mock API response
    server.use(
      rest.get('/api/files/claude-content/*', (req, res, ctx) => {
        return res(ctx.text('# Test Plan\n\nContent here.'));
      })
    );

    render(<TestWrapper floatingContent={{ type: 'markdown', path: '/root/.claude/plans/test.md' }} />);

    await waitFor(() => {
      expect(screen.getByRole('heading')).toHaveTextContent('Test Plan');
    });
  });

  it('shows error state on 404', async () => {
    server.use(
      rest.get('/api/files/claude-content/*', (req, res, ctx) => {
        return res(ctx.status(404), ctx.json({ error: 'File not found' }));
      })
    );

    render(<TestWrapper floatingContent={{ type: 'markdown', path: '/root/.claude/plans/missing.md' }} />);

    await waitFor(() => {
      expect(screen.getByText(/File not found/)).toBeInTheDocument();
    });
  });

  it('shows loading state while fetching', () => {
    render(<TestWrapper floatingContent={{ type: 'markdown', path: '/root/.claude/plans/test.md' }} />);
    expect(screen.getByText(/Loading/)).toBeInTheDocument();
  });
});
```

### Phase 5: E2E Tests

```typescript
// file: e2e/markdown-viewer.spec.ts

import { test, expect } from '@playwright/test';

test.describe('Markdown Viewer', () => {
  test.beforeEach(async ({ page }) => {
    // Create test file
    await page.request.post('/api/test/create-file', {
      data: {
        path: '/root/.claude/plans/e2e-test.md',
        content: '# E2E Test Plan\n\n## Steps\n\n1. Do thing\n2. Check result'
      }
    });
  });

  test('opens markdown viewer with Ctrl+Shift+M after selecting path', async ({ page }) => {
    await page.goto('/');

    // Find terminal iframe and select text containing the path
    const terminal = page.frameLocator('iframe[title^="Terminal"]').first();

    // Simulate having selected "~/.claude/plans/e2e-test.md" in terminal
    // (In real test, would need to echo the path and select it)
    await page.evaluate(() => {
      // Mock selection for testing
      (window as any).__testSelection = '~/.claude/plans/e2e-test.md';
    });

    // Press keyboard shortcut
    await page.keyboard.press('Control+Shift+M');

    // Verify modal appears with content
    await expect(page.locator('.floating-modal')).toBeVisible();
    await expect(page.locator('.markdown-content h1')).toHaveText('E2E Test Plan');
  });

  test('shows error for non-existent file', async ({ page }) => {
    await page.goto('/');

    await page.evaluate(() => {
      (window as any).__testSelection = '~/.claude/plans/nonexistent.md';
    });

    await page.keyboard.press('Control+Shift+M');

    await expect(page.locator('.floating-modal')).toBeVisible();
    await expect(page.locator('.error-message')).toContainText('File not found');
  });

  test('refresh button reloads content', async ({ page }) => {
    await page.goto('/');

    await page.evaluate(() => {
      (window as any).__testSelection = '~/.claude/plans/e2e-test.md';
    });

    await page.keyboard.press('Control+Shift+M');
    await expect(page.locator('.markdown-content h1')).toHaveText('E2E Test Plan');

    // Modify file
    await page.request.post('/api/test/create-file', {
      data: {
        path: '/root/.claude/plans/e2e-test.md',
        content: '# Updated Plan'
      }
    });

    // Click refresh
    await page.locator('button[aria-label="Refresh"]').click();

    await expect(page.locator('.markdown-content h1')).toHaveText('Updated Plan');
  });
});
```

---

## Implementation Order

1. **Backend security** (tests → implementation)
   - `resolveClaudePath()` function with traversal prevention
   - `GET /api/files/claude-content/*` endpoint
   - Run: `go test ./api/...`

2. **Frontend utilities** (tests → implementation)
   - `pathResolver.ts` with `extractMarkdownPath` and `resolveClaudePath`
   - Run: `npm test -- pathResolver`

3. **Markdown rendering** (tests → implementation)
   - `MarkdownContent.tsx` component
   - `MarkdownContent.css` styles
   - Run: `npm test -- MarkdownContent`

4. **Modal integration** (tests → implementation)
   - Extend `FloatingModal` to support markdown mode
   - Add keyboard shortcut in `App.tsx`
   - Run: `npm test -- FloatingModal`

5. **E2E verification**
   - Run: `npx playwright test markdown-viewer`

---

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `api/files.go` | Modify | Add `resolveClaudePath()`, `/claude-content/*` handler |
| `api/files_test.go` | Create | Security and endpoint tests |
| `dashboard/src/utils/pathResolver.ts` | Create | Path extraction and resolution |
| `dashboard/src/utils/pathResolver.test.ts` | Create | Path utility tests |
| `dashboard/src/components/MarkdownContent.tsx` | Create | Markdown rendering |
| `dashboard/src/components/MarkdownContent.css` | Create | Markdown styles |
| `dashboard/src/components/MarkdownContent.test.tsx` | Create | Rendering tests |
| `dashboard/src/components/FloatingModal.tsx` | Modify | Add markdown mode |
| `dashboard/src/context/SessionContext.tsx` | Modify | Add `openMarkdownViewer` action |
| `dashboard/src/App.tsx` | Modify | Add `Ctrl+Shift+M` handler |
| `e2e/markdown-viewer.spec.ts` | Create | E2E tests |
