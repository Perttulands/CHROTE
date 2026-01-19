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

### Primary Flow: Mouse Selection
```
1. User monitors Claude Code session in terminal pane
                    ↓
2. Claude outputs: "Writing plan to ~/.claude/plans/feature-x.md"
                    ↓
3. User selects the path text with mouse (click-drag in terminal)
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

### Alternative Flow: Manual Input
```
1. User presses Ctrl+Shift+M with no selection (or invalid selection)
                    ↓
2. Small input dialog appears: "Enter path to .md file"
                    ↓
3. User pastes or types: ~/.claude/plans/feature-x.md
                    ↓
4. Popup appears with rendered markdown
```

### Important Limitation: tmux Copy Mode

**xterm.js selection ≠ tmux copy mode**

| Selection Method | Works? | Reason |
|-----------------|--------|--------|
| Mouse drag in terminal | ✅ | xterm.js tracks via `term.getSelection()` |
| Double-click word | ✅ | xterm.js handles this |
| tmux copy mode (`Ctrl+B [`) | ❌ | Selection exists in tmux server, not browser |

If users primarily use tmux copy mode, they should use the manual input flow or copy the path to system clipboard first, then use Ctrl+Shift+M.

---

## Design

### Architecture

```
Terminal iframe → User selects text → Ctrl+Shift+M
        ↓
    Access xterm.js via iframe.contentWindow.term.getSelection()
        ↓
    extractMarkdownPath(selection)  →  empty? → show manual input dialog
        ↓
    resolveClaudePath(path)  →  null? → show error "Invalid path"
        ↓
    GET /api/files/claude-content/{path}
        ↓
    403/404/500? → show error in popup
        ↓
    Render markdown in FloatingModal
```

### Security Boundaries

**Problem**: Adding `/root` or `/home/arena` to the existing `allowedRoots` in `files.go` would expose sensitive files (SSH keys, API tokens, configs) to all file operations (read/write/delete).

**Solution**: Scoped read-only endpoint

| Endpoint | Access | Operations |
|----------|--------|------------|
| `/api/files/resources/*` | `/code`, `/vault` | read, write, delete, rename |
| `/api/files/claude-content/*` | `~/.claude/` only | **read only** |

The new endpoint:
- Only allows paths within `~/.claude/` (both `/root/.claude` and `/home/arena/.claude` for Docker/WSL compatibility)
- Read-only (no write/delete/rename)
- Uses `filepath.Rel()` for proper traversal prevention
- 1MB file size limit

### Components

**Backend** (Go - `src/internal/api/files.go`):
- `resolveClaudePath()` - validates path is within `~/.claude/`, prevents traversal
- `GET /api/files/claude-content/*` - returns file content as UTF-8 text

**Frontend** (React - `dashboard/src/`):
- `extractMarkdownPath(text)` - regex extraction of `.md` paths from selection
- `resolveClaudePath(path)` - client-side path normalization (server is source of truth)
- `MarkdownContent` - renders markdown with syntax highlighting, sanitizes `javascript:` links
- Extended `FloatingModal` - reuses existing draggable modal, adds markdown mode

---

## Test-Driven Development Plan

### Phase 1: Backend Security Tests

Write tests first, then implement `resolveClaudePath()` and the endpoint.

**File:** `src/internal/api/files_test.go`

```go
func TestResolveClaudePath(t *testing.T) {
	h := NewFilesHandler()

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
			result := h.resolveClaudePath(tt.input)
			if tt.wantErr {
				if result.Error == "" {
					t.Errorf("expected error containing %q, got success", tt.errMsg)
				} else if !strings.Contains(result.Error, tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, result.Error)
				}
			} else {
				if result.Error != "" {
					t.Errorf("unexpected error: %s", result.Error)
				}
				if result.Path == "" {
					t.Error("expected non-empty path")
				}
			}
		})
	}
}

func TestClaudeContentEndpoint(t *testing.T) {
	// Setup test directory
	testDir := t.TempDir()
	claudeDir := filepath.Join(testDir, ".claude", "plans")
	os.MkdirAll(claudeDir, 0755)

	// Create test file
	testContent := "# Test Plan\n\nThis is a test."
	testFile := filepath.Join(claudeDir, "test.md")
	os.WriteFile(testFile, []byte(testContent), 0644)

	// Create large file (2MB)
	largeFile := filepath.Join(claudeDir, "large.md")
	os.WriteFile(largeFile, make([]byte, 2*1024*1024), 0644)

	h := &FilesHandler{
		claudeHomes: []string{testDir}, // Use temp dir as claude home for testing
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/claude-content/{path...}", h.GetClaudeContent)

	t.Run("returns 403 for path outside .claude", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/claude-content/etc/passwd", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})

	t.Run("returns 404 for nonexistent file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/claude-content"+testDir+"/.claude/plans/nonexistent.md", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("returns 413 for file over 1MB", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/claude-content"+testDir+"/.claude/plans/large.md", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413, got %d", w.Code)
		}
	})

	t.Run("returns content for valid file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/files/claude-content"+testDir+"/.claude/plans/test.md", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if w.Body.String() != testContent {
			t.Errorf("expected %q, got %q", testContent, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("expected text/plain content-type, got %s", ct)
		}
	})
}
```

### Phase 2: Frontend Path Resolution Tests

**File:** `dashboard/src/utils/pathResolver.test.ts`

```typescript
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

**File:** `dashboard/src/components/MarkdownContent.test.tsx`

```typescript
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

**File:** `dashboard/src/components/FloatingModal.test.tsx`

```typescript
describe('FloatingModal markdown mode', () => {
  it('fetches and displays markdown content', async () => {
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
});
```

---

## Implementation Order

1. **Backend security** (tests → implementation)
   - Add `resolveClaudePath()` to `src/internal/api/files.go`
   - Add `GET /api/files/claude-content/*` handler
   - Run: `go test ./src/internal/api/...`

2. **Frontend utilities** (tests → implementation)
   - Create `dashboard/src/utils/pathResolver.ts`
   - Run: `npm test -- pathResolver`

3. **Markdown rendering** (tests → implementation)
   - Create `dashboard/src/components/MarkdownContent.tsx`
   - Create `dashboard/src/components/MarkdownContent.css`
   - Run: `npm test -- MarkdownContent`

4. **Modal integration** (tests → implementation)
   - Extend `dashboard/src/components/FloatingModal.tsx` for markdown mode
   - Add keyboard shortcut in `dashboard/src/App.tsx`
   - Run: `npm test -- FloatingModal`

### Keyboard Handler Implementation

**File:** `dashboard/src/App.tsx`

The key insight: ttyd exposes xterm.js as `window.term` in the iframe. We access it via `iframe.contentWindow.term.getSelection()`.

```typescript
// Type for ttyd's exposed xterm instance
type TtydWindow = Window & {
  term?: {
    getSelection: () => string;
    hasSelection: boolean;
    options: { fontSize: number };
  };
};

// In App.tsx or a custom hook
useEffect(() => {
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.ctrlKey && e.shiftKey && e.key === 'M') {
      e.preventDefault();

      let selection = '';

      // Try to get selection from all terminal iframes
      const iframes = document.querySelectorAll<HTMLIFrameElement>('iframe[title^="Terminal"]');
      for (const iframe of iframes) {
        try {
          const win = iframe.contentWindow as TtydWindow;
          if (win?.term?.hasSelection) {
            selection = win.term.getSelection();
            if (selection) break;
          }
        } catch {
          // Cross-origin or not ready - continue
        }
      }

      // Also check the floating modal iframe
      const floatingIframe = document.querySelector<HTMLIFrameElement>('.floating-modal iframe');
      if (!selection && floatingIframe) {
        try {
          const win = floatingIframe.contentWindow as TtydWindow;
          if (win?.term?.hasSelection) {
            selection = win.term.getSelection();
          }
        } catch {
          // Ignore
        }
      }

      if (selection) {
        const path = extractMarkdownPath(selection);
        if (path) {
          const resolved = resolveClaudePath(path);
          if (resolved) {
            openMarkdownViewer(resolved);
            return;
          }
        }
      }

      // No valid selection - show manual input dialog
      openMarkdownInputDialog();
    }
  };

  window.addEventListener('keydown', handleKeyDown);
  return () => window.removeEventListener('keydown', handleKeyDown);
}, [openMarkdownViewer, openMarkdownInputDialog]);
```

5. **E2E verification**
   - Run: `npx playwright test markdown-viewer`

---

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `src/internal/api/files.go` | Modify | Add `resolveClaudePath()`, `GetClaudeContent()` handler |
| `src/internal/api/files_test.go` | Modify | Add security and endpoint tests |
| `dashboard/src/utils/pathResolver.ts` | Create | Path extraction and resolution |
| `dashboard/src/utils/pathResolver.test.ts` | Create | Path utility tests |
| `dashboard/src/components/MarkdownContent.tsx` | Create | Markdown rendering |
| `dashboard/src/components/MarkdownContent.css` | Create | Markdown styles |
| `dashboard/src/components/MarkdownContent.test.tsx` | Create | Rendering tests |
| `dashboard/src/components/FloatingModal.tsx` | Modify | Add markdown mode |
| `dashboard/src/context/SessionContext.tsx` | Modify | Add `openMarkdownViewer` action |
| `dashboard/src/App.tsx` | Modify | Add `Ctrl+Shift+M` handler |

---

## Dependencies to Install

```bash
cd dashboard && npm install react-markdown remark-gfm react-syntax-highlighter @types/react-syntax-highlighter
```
