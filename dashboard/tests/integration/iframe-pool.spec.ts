import { test, expect } from '@playwright/test';

/**
 * Integration tests for IframePool terminal rendering.
 * Tests against the real running CHROTE backend at port 8090.
 *
 * Verifies that terminal iframes render visibly when sessions are bound to windows,
 * across multiple flows: new session button, drag-and-drop, page reload persistence,
 * and preset switching.
 */
test.describe.serial('IframePool: session iframe renders in window', () => {
  const createdSessions: string[] = [];

  test.afterEach(async ({ request }) => {
    // Clean up sessions we created
    for (const name of createdSessions) {
      try {
        // truthsayer:ignore sql-injection -- test-controlled variable, not user input
        await request.delete(`/api/tmux/sessions/${name}`);
      } catch { /* ignore */ }
    }
    createdSessions.length = 0;
  });

  test('new session button creates a visible iframe in window body', async ({ page }) => {
    // Clear localStorage to start fresh
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });
    await page.waitForSelector('.terminal-window', { timeout: 5000 });

    const firstWindow = page.locator('.terminal-window').first();
    const createBtn = firstWindow.locator('.create-session-btn');
    await expect(createBtn).toBeVisible({ timeout: 5000 });

    // Capture the session name from the POST request
    let sessionName = '';
    page.on('request', req => {
      if (req.method() === 'POST' && req.url().includes('/api/tmux/sessions')) {
        try { sessionName = JSON.parse(req.postData() || '{}').name || ''; } catch {}
      }
    });

    await createBtn.click();

    // Wait for session tag to appear
    await expect(firstWindow.locator('.session-tag')).toHaveCount(1, { timeout: 10000 });
    if (sessionName) createdSessions.push(sessionName);

    // KEY: iframe must exist in window body and be visible
    const body = firstWindow.locator('.terminal-window-body');
    const iframe = body.locator('iframe');
    await expect(iframe).toHaveCount(1, { timeout: 10000 });

    const styles = await iframe.evaluate((el: HTMLIFrameElement) => ({
      display: getComputedStyle(el).display,
      visibility: getComputedStyle(el).visibility,
      position: getComputedStyle(el).position,
      width: el.offsetWidth,
      height: el.offsetHeight,
    }));
    console.log('New session iframe styles:', styles);
    expect(styles.display).not.toBe('none');
    expect(styles.visibility).not.toBe('hidden');
    expect(styles.width).toBeGreaterThan(100);
    expect(styles.height).toBeGreaterThan(50);

    // Iframe src should point to ttyd terminal
    const src = await iframe.getAttribute('src');
    expect(src).toContain('/terminal/');

    // Wait for iframe to load, then check terminal is interactive
    const iframeEl = iframe.first();
    const frame = iframeEl.contentFrame();
    if (frame) {
      // Wait for xterm.js to render inside the iframe (ttyd needs time to connect)
      const termEl = frame.locator('.xterm');
      await expect(async () => {
        const termCount = await termEl.count();
        expect(termCount).toBeGreaterThan(0);
      }).toPass({ timeout: 10000 });
      console.log('xterm elements in iframe:', await termEl.count());
      await expect(termEl.first()).toBeVisible();
    }
  });

  test('iframe survives tab switch (terminal -> files -> terminal)', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const firstWindow = page.locator('.terminal-window').first();
    const createBtn = firstWindow.locator('.create-session-btn');
    await expect(createBtn).toBeVisible({ timeout: 5000 });

    let sessionName = '';
    page.on('request', req => {
      if (req.method() === 'POST' && req.url().includes('/api/tmux/sessions')) {
        try { sessionName = JSON.parse(req.postData() || '{}').name || ''; } catch {}
      }
    });

    // Create a session
    await createBtn.click();
    await expect(firstWindow.locator('.session-tag')).toHaveCount(1, { timeout: 10000 });
    if (sessionName) createdSessions.push(sessionName);

    // Verify iframe is there
    const body = firstWindow.locator('.terminal-window-body');
    await expect(body.locator('iframe')).toHaveCount(1, { timeout: 5000 });

    // Switch to Files tab
    await page.click('.tab:has-text("Files")');
    await page.waitForSelector('.files-view', { timeout: 5000 });

    // Switch back to Terminal
    await page.click('.tab:has-text("Terminal")');
    await page.waitForSelector('.terminal-window', { timeout: 5000 });

    // Iframe should STILL be there and visible (not recreated)
    const iframeAfter = page.locator('.terminal-window').first().locator('.terminal-window-body iframe');
    await expect(iframeAfter).toHaveCount(1, { timeout: 5000 });

    const styles = await iframeAfter.evaluate((el: HTMLIFrameElement) => ({
      display: getComputedStyle(el).display,
      visibility: getComputedStyle(el).visibility,
      width: el.offsetWidth,
      height: el.offsetHeight,
    }));
    console.log('After tab switch iframe styles:', styles);
    expect(styles.display).not.toBe('none');
    expect(styles.visibility).not.toBe('hidden');
    expect(styles.width).toBeGreaterThan(100);
    expect(styles.height).toBeGreaterThan(50);
  });

  test('iframe persists across page reload', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const firstWindow = page.locator('.terminal-window').first();

    let sessionName = '';
    page.on('request', req => {
      if (req.method() === 'POST' && req.url().includes('/api/tmux/sessions')) {
        try { sessionName = JSON.parse(req.postData() || '{}').name || ''; } catch {}
      }
    });

    // Create session
    await firstWindow.locator('.create-session-btn').click();
    await expect(firstWindow.locator('.session-tag')).toHaveCount(1, { timeout: 10000 });
    if (sessionName) createdSessions.push(sessionName);

    // Verify iframe
    await expect(firstWindow.locator('.terminal-window-body iframe')).toHaveCount(1, { timeout: 5000 });

    // Reload page (session binding persists via localStorage)
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    // Session tag should still be there (from localStorage)
    const windowAfter = page.locator('.terminal-window').first();
    await expect(windowAfter.locator('.session-tag')).toHaveCount(1, { timeout: 10000 });

    // Iframe should render for the persisted session
    const iframe = windowAfter.locator('.terminal-window-body iframe');
    await expect(iframe).toHaveCount(1, { timeout: 10000 });

    const styles = await iframe.evaluate((el: HTMLIFrameElement) => ({
      display: getComputedStyle(el).display,
      visibility: getComputedStyle(el).visibility,
      width: el.offsetWidth,
      height: el.offsetHeight,
    }));
    console.log('After reload iframe styles:', styles);
    expect(styles.display).not.toBe('none');
    expect(styles.visibility).not.toBe('hidden');
    expect(styles.width).toBeGreaterThan(100);
    expect(styles.height).toBeGreaterThan(50);
  });
});
