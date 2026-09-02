import { test, expect, Page, Locator, APIRequestContext } from '@playwright/test';
import {
  cleanupTrackedSessions as reconcileTrackedSessions,
  LiveSessionIdentity,
} from '../helpers/liveSessionCleanup';

/**
 * Integration tests for terminal sizing against the real CHROTE backend.
 *
 * The terminal is rendered in the dashboard's own document (ADR-0018), so the
 * grid, the viewport and the rendered rows are all directly observable.
 *
 * Beads: pol-8dca, pol-3798, pol-1c0b, chrote-qlx, chrote-jkzk.1
 */
test.describe.serial('Terminal sizing: the pane fits its frame', () => {
  test.describe.configure({ retries: 0 });
  const createdSessions: LiveSessionIdentity[] = [];

  function visibleArea(page: Page) {
    return page.locator('.terminal-grid[data-workspace="terminal1"]');
  }

  function layoutControls(page: Page) {
    return page.locator('.terminal-area-controls').first();
  }

  function terminal(page: Page): Locator {
    return page.locator('.terminal-window-body .terminal-surface').first();
  }

  async function waitForTerminal(page: Page, timeout = 15000): Promise<void> {
    await terminal(page).locator('.xterm').waitFor({ state: 'visible', timeout });
  }

  /** The rendered rows must not overflow the viewport they sit in. */
  async function expectNotClipped(page: Page, timeout = 5000): Promise<void> {
    await expect(async () => {
      const clipped = await terminal(page).evaluate(surface => {
        const viewport = surface.querySelector('.xterm-viewport') as HTMLElement | null;
        const screen = surface.querySelector('.xterm-screen') as HTMLElement | null;
        if (!viewport || !screen) return true;
        return screen.scrollHeight > viewport.clientHeight + 5;
      });
      expect(clipped).toBe(false);
    }).toPass({ timeout });
  }

  const rowsText = (page: Page) => terminal(page).locator('.xterm-rows').innerText();

  /**
   * Type a marker until the shell echoes it. A freshly created session can
   * swallow keystrokes typed before its pty is ready, so retype on a fresh
   * line when nothing echoes.
   */
  async function typeMarkerUntilEchoed(page: Page, marker: string, attempts = 5): Promise<void> {
    const input = terminal(page).locator('.xterm-helper-textarea');
    for (let attempt = 1; attempt <= attempts; attempt++) {
      await input.press('Control+u');
      await input.pressSequentially(marker);
      try {
        await expect(async () => {
          expect(await rowsText(page)).toContain(marker);
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
    // A shell, not an agent: these journeys assert plain terminal output, and
    // the launcher's first harness is whatever the host configured.
    await terminalWindow.locator('.launcher-row', { hasText: 'Shell' }).click();
    await terminalWindow.locator('.launcher-launch').click();
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
    // The Refit button is never clicked: the automatic path must recover the
    // geometry on its own (chrote-qlx acceptance).
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    await layoutControls(page).locator('.layout-btn').filter({ hasText: '1' }).click();
    const firstWindow = visibleArea(page).locator('.terminal-window').first();

    await createTrackedSession(page, firstWindow);
    await waitForTerminal(page);

    const promptMarker = 'CHROTE_AUTOFIT_INPUT_MARKER';
    await typeMarkerUntilEchoed(page, promptMarker);

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
    await waitForTerminal(page);
    await expectNotClipped(page);

    // The marker's own row must sit entirely inside the visible viewport:
    // a grid left at the old geometry pushes the input row out of frame.
    await expect(async () => {
      const placement = await terminal(page).evaluate(marker => {
        const viewport = surfaceViewport();
        const row = rowContaining(marker);
        if (!viewport || !row) return null;
        const viewportRect = viewport.getBoundingClientRect();
        const rowRect = row.getBoundingClientRect();
        return {
          fullyVisible: rowRect.top >= viewportRect.top - 1
            && rowRect.bottom <= viewportRect.bottom + 1
            && rowRect.left >= viewportRect.left - 1
            && rowRect.right <= viewportRect.right + 1,
        };

        function surfaceViewport(): HTMLElement | null {
          return document.querySelector('.terminal-window-body .terminal-surface .xterm-viewport');
        }
        function rowContaining(text: string): HTMLElement | null {
          const rows = document.querySelectorAll<HTMLElement>('.terminal-window-body .terminal-surface .xterm-rows > div');
          return Array.from(rows).find(candidate => candidate.textContent?.includes(text)) ?? null;
        }
      }, promptMarker);

      expect(placement, 'the marker row must still be rendered').not.toBeNull();
      expect(placement!.fullyVisible).toBe(true);
    }).toPass({ timeout: 5000 });
  });
});
