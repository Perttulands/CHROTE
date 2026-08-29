import { test, expect, Page, Frame, Locator, APIRequestContext } from '@playwright/test';
import {
  cleanupTrackedSessions as reconcileTrackedSessions,
  LiveSessionIdentity,
} from '../helpers/liveSessionCleanup';

/**
 * Integration tests for terminal iframe sizing and fit behavior.
 * Tests against the real CHROTE backend.
 *
 * Verifies that:
 * - Terminal iframes fill their container completely
 * - xterm.js fit() is triggered after iframe load (input prompt visible)
 * - Terminals re-fit correctly on layout changes and tab switches
 *
 * Beads: pol-8dca, pol-3798, pol-1c0b
 */
test.describe.serial('Terminal Sizing: iframe fills container and xterm fits', () => {
  test.describe.configure({ retries: 0 });
  const createdSessions: LiveSessionIdentity[] = [];

  // Helper: get the visible terminal grid (scoped to active workspace)
  function visibleArea(page: Page) {
    return page.locator('.terminal-grid[data-workspace="terminal1"]');
  }

  // Helper: get the visible layout controls
  function layoutControls(page: Page) {
    return page.locator('.terminal-area-controls').first();
  }

  // Helper: find the terminal iframe's Frame object for evaluate() calls
  function getTerminalFrame(page: Page): Frame | null {
    return page.frames().find(f => f.url().includes('/terminal/')) ?? null;
  }

  // Helper: wait for xterm to be ready inside terminal iframe
  async function waitForXterm(page: Page, timeout = 10000): Promise<Frame> {
    const frameLocator = page.locator('.terminal-window-body iframe').first().contentFrame();
    await frameLocator.locator('.xterm').waitFor({ state: 'visible', timeout });
    const frame = getTerminalFrame(page);
    if (!frame) throw new Error('Terminal iframe frame not found');
    return frame;
  }

  // Helper: wait for xterm fit() to complete (viewport contains screen without clipping)
  async function waitForFit(frame: Frame, timeout = 5000): Promise<void> {
    await expect(async () => {
      const fitted = await frame.evaluate(() => {
        const viewport = document.querySelector('.xterm-viewport') as HTMLElement;
        const screen = document.querySelector('.xterm-screen') as HTMLElement;
        if (!viewport || !screen) return false;
        return screen.scrollHeight <= viewport.clientHeight + 5;
      });
      expect(fitted).toBe(true);
    }).toPass({ timeout });
  }

  // Helper: type a marker until the shell echoes it. A freshly created
  // session can swallow keystrokes typed before its pty/shell is ready, so
  // retype on a fresh line when nothing echoes. Each attempt gives the echo
  // time to round-trip before reading — clearing and retyping inside a tight
  // read loop would erase the echo and re-race it forever.
  async function typeMarkerUntilEchoed(frame: Frame, marker: string, attempts = 5): Promise<void> {
    const readCursorLine = () => frame.evaluate(() => {
      const term = (window as typeof window & { term?: { buffer: { active: { baseY: number; cursorY: number; getLine: (row: number) => { translateToString: (trimRight?: boolean) => string } | undefined } } } }).term;
      // Cursor line is baseY + cursorY: cursorY alone indexes scrollback
      // whenever the buffer has scrolled or shrunk (baseY > 0).
      return term?.buffer.active.getLine(term.buffer.active.baseY + term.buffer.active.cursorY)?.translateToString(true) ?? '';
    });
    for (let attempt = 1; attempt <= attempts; attempt++) {
      await frame.locator('.xterm-helper-textarea').press('Control+u');
      await frame.locator('.xterm-helper-textarea').pressSequentially(marker);
      try {
        await expect(async () => {
          expect(await readCursorLine()).toContain(marker);
        }).toPass({ timeout: 2000, intervals: [100, 250, 500] });
        return;
      } catch {
        // Keystrokes swallowed (pty/shell not ready yet) — retype.
      }
    }
    throw new Error(`marker never echoed after ${attempts} typing attempts`);
  }

  async function createTrackedSession(page: Page, terminalWindow: Locator): Promise<string> {
    const responsePromise = page.waitForResponse(response =>
      response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/tmux/sessions'
    );
    await terminalWindow.locator('.create-session-btn').click();
    const response = await responsePromise;
    expect(response.ok(), await response.text()).toBe(true);
    const payload = await response.json() as { session?: string };
    const requestPayload = response.request().postDataJSON() as { unixUser?: string } | null;
    expect(payload.session).toBeTruthy();
    createdSessions.push({ name: payload.session!, unixUser: requestPayload?.unixUser });
    await expect(terminalWindow.locator('.session-tag')).toHaveCount(1, { timeout: 10000 });
    return payload.session!;
  }

  async function cleanupTrackedSessions(request: APIRequestContext, attempts: number): Promise<string[]> {
    return reconcileTrackedSessions(createdSessions, async session => {
      const query = session.unixUser ? `?unixUser=${encodeURIComponent(session.unixUser)}` : '';
      const response = await request.delete(`/api/tmux/sessions/${encodeURIComponent(session.name)}${query}`);
      return {
        ok: response.ok(),
        status: response.status(),
        body: response.ok() ? '' : await response.text(),
      };
    }, attempts);
  }

  test.afterEach(async ({ request }) => {
    const failures = await cleanupTrackedSessions(request, 2);
    expect(failures, 'every live sizing smoke session must be deleted; unresolved unixUser/session tuples remain tracked').toEqual([]);
  });

  test.afterAll(async ({ request }) => {
    const failures = await cleanupTrackedSessions(request, 3);
    expect(failures, 'final live sizing reconciliation left unresolved unixUser/session tuples').toEqual([]);
  });

  test('input marker stays visible after a hidden-tab viewport change WITHOUT manual Refit', async ({ page }) => {
    // No-Refit variant (chrote-qlx acceptance): the closed-loop fit path —
    // font-then-fit on load plus ResizeObserver-driven term.fit() — must
    // recover the final geometry on its own. The Refit button is never
    // clicked anywhere in this test.
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const controls = layoutControls(page);
    await controls.locator('.layout-btn').filter({ hasText: '1' }).click();
    const firstWindow = visibleArea(page).locator('.terminal-window').first();

    await createTrackedSession(page, firstWindow);

    const termFrameBefore = await waitForXterm(page);
    const promptMarker = 'CHROTE_AUTOFIT_INPUT_MARKER';
    await typeMarkerUntilEchoed(termFrameBefore, promptMarker);

    // Hide Terminal behind Files, change viewport geometry while hidden.
    await page.click('.tab:has-text("Files")');
    await expect(page.getByRole('heading', { name: 'Files' })).toBeVisible({ timeout: 5000 });
    const originalViewport = page.viewportSize();
    if (originalViewport) {
      await page.setViewportSize({
        width: originalViewport.width > 1000 ? originalViewport.width - 180 : originalViewport.width + 180,
        height: originalViewport.height > 640 ? originalViewport.height - 100 : originalViewport.height + 100,
      });
    }

    // Return to Terminal 1 and let the automatic paths settle — no Refit.
    await page.click('.tab:has-text("Terminal")');
    await page.waitForSelector('.terminal-window', { timeout: 5000 });
    const termFrameAfter = getTerminalFrame(page);
    if (!termFrameAfter) throw new Error('Terminal iframe disappeared after tab return');
    await waitForFit(termFrameAfter);

    await expect(async () => {
      const state = await termFrameAfter.evaluate(marker => {
        const viewport = document.querySelector('.xterm-viewport') as HTMLElement;
        const screen = document.querySelector('.xterm-screen') as HTMLElement;
        const term = (window as typeof window & { term?: { cols: number; rows: number; buffer: { active: { baseY: number; cursorY: number; getLine: (row: number) => { translateToString: (trimRight?: boolean) => string } | undefined } } } }).term;
        const renderCanvas = Array.from(screen?.querySelectorAll('canvas') ?? [])
          .find(canvas => !canvas.classList.contains('xterm-link-layer'));
        if (!viewport || !screen || !term || !renderCanvas) return null;

        const inputLine = term.buffer.active.getLine(term.buffer.active.baseY + term.buffer.active.cursorY)?.translateToString(true) ?? '';
        const markerColumn = inputLine.indexOf(marker);
        const viewportRect = viewport.getBoundingClientRect();
        const canvasRect = renderCanvas.getBoundingClientRect();
        const cellWidthCss = canvasRect.width / term.cols;
        const cellHeightCss = canvasRect.height / term.rows;
        const markerBottom = canvasRect.top + (term.buffer.active.cursorY + 1) * cellHeightCss;
        const markerTop = canvasRect.top + term.buffer.active.cursorY * cellHeightCss;
        const markerLeft = canvasRect.left + markerColumn * cellWidthCss;
        const markerRight = canvasRect.left + (markerColumn + marker.length) * cellWidthCss;

        return {
          isClipped: screen.scrollHeight > viewport.clientHeight + 5,
          inputLine,
          markerFullyVisible: markerColumn >= 0
            && markerLeft >= viewportRect.left
            && markerRight <= viewportRect.right
            && markerTop >= viewportRect.top
            && markerBottom <= viewportRect.bottom,
        };
      }, promptMarker);

      expect(state).not.toBeNull();
      expect(state!.isClipped).toBe(false);
      expect(state!.inputLine).toContain(promptMarker);
      expect(state!.markerFullyVisible).toBe(true);
    }).toPass({ timeout: 5000 });
  });
});
