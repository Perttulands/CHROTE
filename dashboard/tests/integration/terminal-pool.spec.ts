import { test, expect, APIRequestContext, Locator, Page } from '@playwright/test';
import {
  cleanupTrackedSessions as reconcileTrackedSessions,
  LiveSessionIdentity,
} from '../helpers/liveSessionCleanup';

/**
 * Integration tests for the pooled in-page terminal, against a real CHROTE
 * backend. Verifies that a session bound to a window actually attaches and
 * renders shell output, and that the binding still does so after a reload.
 */
test.describe.serial('terminal pool: a bound session renders in its window', () => {
  test.describe.configure({ retries: 0 });
  const createdSessions: LiveSessionIdentity[] = [];

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

  /** The terminal must be attached, on screen, and showing what the shell wrote. */
  async function expectLiveTerminal(terminalWindow: Locator) {
    const terminal = terminalWindow.locator('.terminal-window-body .terminal-surface .xterm');
    await expect(terminal).toBeVisible({ timeout: 15000 });
    await expect(terminalWindow.locator('.terminal-loading-state')).toHaveCount(0, { timeout: 15000 });
    await expect(async () => {
      const box = await terminal.boundingBox();
      expect(box?.width ?? 0).toBeGreaterThan(100);
      expect(box?.height ?? 0).toBeGreaterThan(50);
    }).toPass({ timeout: 10000 });
    await expect(terminalWindow.locator('.xterm-rows')).not.toBeEmpty({ timeout: 15000 });
  }

  async function freshDashboard(page: Page) {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });
    await page.waitForSelector('.terminal-window', { timeout: 5000 });
  }

  test.afterEach(async ({ request }) => {
    const failures = await cleanupTrackedSessions(request, 2);
    expect(failures, 'every terminal-pool smoke session must be deleted; unresolved unixUser/session tuples remain tracked').toEqual([]);
  });

  test.afterAll(async ({ request }) => {
    const failures = await cleanupTrackedSessions(request, 3);
    expect(failures, 'final terminal-pool reconciliation left unresolved unixUser/session tuples').toEqual([]);
  });

  test('a session created from the window button attaches and renders', async ({ page }) => {
    await freshDashboard(page);

    const firstWindow = page.locator('.terminal-window').first();
    await expect(firstWindow.locator('.create-session-btn')).toBeVisible({ timeout: 5000 });

    await createTrackedSession(page, firstWindow);

    await expectLiveTerminal(firstWindow);
  });

  test('the binding still renders a live terminal after a reload', async ({ page }) => {
    await freshDashboard(page);

    const firstWindow = page.locator('.terminal-window').first();
    await createTrackedSession(page, firstWindow);
    await expectLiveTerminal(firstWindow);

    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const windowAfter = page.locator('.terminal-window').first();
    await expect(windowAfter.locator('.session-tag')).toHaveCount(1, { timeout: 10000 });
    await expectLiveTerminal(windowAfter);
  });
});
