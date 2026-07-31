import { test, expect, Page, Frame, Locator } from '@playwright/test';

type RefitProbeWindow = typeof window & {
  __chroteRefitProbe?: {
    events: Array<{ at: number; width: number; height: number }>;
    installed: boolean;
  };
};

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
  const createdSessions: Array<{ name: string; unixUser?: string }> = [];

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

  // Helper: wait for iframe to reach stable dimensions after resize
  async function waitForIframeResize(iframe: ReturnType<typeof page.locator>, prevWidth: number, timeout = 5000): Promise<void> {
    await expect(async () => {
      const dims = await iframe.evaluate((el: HTMLIFrameElement) => el.offsetWidth);
      expect(dims).not.toBe(prevWidth);
    }).toPass({ timeout });
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

  test.afterEach(async ({ request }) => {
    const cleanupFailures: string[] = [];
    for (const session of createdSessions) {
      try {
        const query = session.unixUser ? `?unixUser=${encodeURIComponent(session.unixUser)}` : '';
        const response = await request.delete(`/api/tmux/sessions/${encodeURIComponent(session.name)}${query}`);
        if (!response.ok()) cleanupFailures.push(`${session.name}: HTTP ${response.status()} ${await response.text()}`);
      } catch (error) {
        cleanupFailures.push(`${session.name}: ${String(error)}`);
      }
    }
    createdSessions.length = 0;
    expect(cleanupFailures, 'every live sizing smoke session must be deleted').toEqual([]);
  });

  test('iframe dimensions match container after session load', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });
    await page.waitForSelector('.terminal-window', { timeout: 5000 });

    // Ensure single-window layout
    const controls = layoutControls(page);
    await controls.locator('.layout-btn').filter({ hasText: '1' }).click();

    const firstWindow = visibleArea(page).locator('.terminal-window').first();
    const createBtn = firstWindow.locator('.create-session-btn');
    await expect(createBtn).toBeVisible({ timeout: 5000 });

    await createTrackedSession(page, firstWindow);

    // Wait for iframe + xterm
    const body = firstWindow.locator('.terminal-window-body');
    await expect(body.locator('iframe')).toHaveCount(1, { timeout: 10000 });
    const termFrame1 = await waitForXterm(page);
    await waitForFit(termFrame1);

    // KEY TEST: iframe must fill its container
    const sizing = await page.evaluate(() => {
      const bodies = document.querySelectorAll('.terminal-window-body');
      // Find the first body that has an iframe with display:block
      for (const b of Array.from(bodies)) {
        const iframe = b.querySelector('iframe') as HTMLIFrameElement;
        if (!iframe || getComputedStyle(iframe).display === 'none') continue;
        const body = b as HTMLElement;
        return {
          containerWidth: body.clientWidth,
          containerHeight: body.clientHeight,
          iframeWidth: iframe.offsetWidth,
          iframeHeight: iframe.offsetHeight,
          iframeOverflowsWidth: iframe.offsetWidth > body.clientWidth + 2,
          iframeOverflowsHeight: iframe.offsetHeight > body.clientHeight + 2,
        };
      }
      return null;
    });

    console.log('Sizing after load:', sizing);
    expect(sizing).not.toBeNull();
    expect(sizing!.iframeWidth).toBeGreaterThan(100);
    expect(sizing!.iframeHeight).toBeGreaterThan(100);
    expect(sizing!.iframeWidth).toBeGreaterThanOrEqual(sizing!.containerWidth - 2);
    expect(sizing!.iframeHeight).toBeGreaterThanOrEqual(sizing!.containerHeight - 2);
    expect(sizing!.iframeOverflowsWidth).toBe(false);
    expect(sizing!.iframeOverflowsHeight).toBe(false);
  });

  test('xterm is fitted to visible area after iframe load (no clipping)', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const controls = layoutControls(page);
    await controls.locator('.layout-btn').filter({ hasText: '1' }).click();

    const firstWindow = visibleArea(page).locator('.terminal-window').first();
    await expect(firstWindow.locator('.create-session-btn')).toBeVisible({ timeout: 5000 });

    await createTrackedSession(page, firstWindow);

    // Wait for xterm to render and fit
    const termFrame = await waitForXterm(page);
    await waitForFit(termFrame);

    // Check that xterm's viewport doesn't overflow its container
    const xtermSizing = await termFrame.evaluate(() => {
      const xtermEl = document.querySelector('.xterm') as HTMLElement;
      const viewport = document.querySelector('.xterm-viewport') as HTMLElement;
      const screen = document.querySelector('.xterm-screen') as HTMLElement;
      if (!xtermEl || !viewport || !screen) return null;

      return {
        xtermHeight: xtermEl.clientHeight,
        viewportHeight: viewport.clientHeight,
        screenHeight: screen.clientHeight,
        screenScrollHeight: screen.scrollHeight,
        // If screen is taller than viewport, content is clipped (prompt may be hidden)
        isClipped: screen.scrollHeight > viewport.clientHeight + 5,
        viewportScrollTop: viewport.scrollTop,
        viewportScrollHeight: viewport.scrollHeight,
      };
    });

    console.log('xterm sizing after load:', xtermSizing);
    expect(xtermSizing).not.toBeNull();

    // The xterm screen should fit within the viewport (no clipping)
    // If this fails, fit() wasn't called or calculated wrong dimensions
    expect(xtermSizing!.isClipped).toBe(false);
  });

  test('terminal re-fits correctly when changing window count', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const controls = layoutControls(page);
    await controls.locator('.layout-btn').filter({ hasText: '1' }).click();

    const firstWindow = visibleArea(page).locator('.terminal-window').first();

    await createTrackedSession(page, firstWindow);

    const iframe = firstWindow.locator('.terminal-window-body iframe').first();
    const termFrame = await waitForXterm(page);
    await waitForFit(termFrame);

    // Record dimensions at 1 window
    const sizeAt1 = await iframe.evaluate((el: HTMLIFrameElement) => ({
      width: el.offsetWidth,
      height: el.offsetHeight,
    }));

    // Switch to 2 windows - terminal should get smaller
    await controls.locator('.layout-btn').filter({ hasText: '2' }).click();
    await waitForIframeResize(iframe, sizeAt1.width);

    const sizeAt2 = await iframe.evaluate((el: HTMLIFrameElement) => ({
      width: el.offsetWidth,
      height: el.offsetHeight,
    }));

    console.log('Size at 1 window:', sizeAt1, 'Size at 2 windows:', sizeAt2);
    expect(sizeAt2.width).toBeLessThan(sizeAt1.width);

    // After resize, xterm should still fit (no clipping)
    const xtermSizing = await termFrame.evaluate(() => {
      const viewport = document.querySelector('.xterm-viewport') as HTMLElement;
      const screen = document.querySelector('.xterm-screen') as HTMLElement;
      if (!viewport || !screen) return null;
      return {
        isClipped: screen.scrollHeight > viewport.clientHeight + 5,
        viewportHeight: viewport.clientHeight,
        screenScrollHeight: screen.scrollHeight,
      };
    });

    console.log('xterm sizing after resize:', xtermSizing);
    expect(xtermSizing).not.toBeNull();
    expect(xtermSizing!.isClipped).toBe(false);
  });

  test('terminal re-fits after a hidden tab viewport change and renders the input marker visibly', async ({ page }) => {
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.waitForSelector('.dashboard', { timeout: 10000 });

    const controls = layoutControls(page);
    await controls.locator('.layout-btn').filter({ hasText: '1' }).click();
    const firstWindow = visibleArea(page).locator('.terminal-window').first();

    await createTrackedSession(page, firstWindow);

    const termFrameBefore = await waitForXterm(page);
    const promptMarker = 'CHROTE_REFIT_INPUT_MARKER';
    await termFrameBefore.locator('.xterm-helper-textarea').pressSequentially(promptMarker);
    await expect(async () => {
      const line = await termFrameBefore.evaluate(() => {
        const term = (window as typeof window & { term?: { buffer: { active: { cursorY: number; getLine: (row: number) => { translateToString: (trimRight?: boolean) => string } | undefined } } } }).term;
        return term?.buffer.active.getLine(term.buffer.active.cursorY)?.translateToString(true) ?? '';
      });
      expect(line).toContain(promptMarker);
    }).toPass({ timeout: 5000 });
    await termFrameBefore.evaluate(() => {
      const probeWindow = window as RefitProbeWindow;
      probeWindow.__chroteRefitProbe ??= { events: [], installed: false };
      if (!probeWindow.__chroteRefitProbe.installed) {
        window.addEventListener('resize', event => {
          if (event.isTrusted) return;
          probeWindow.__chroteRefitProbe?.events.push({
            at: performance.now(),
            width: window.innerWidth,
            height: window.innerHeight,
          });
        });
        probeWindow.__chroteRefitProbe.installed = true;
      }
    });

    // Hide Terminal behind Files, then change viewport geometry while it is hidden.
    await page.click('.tab:has-text("Files")');
    await expect(page.getByRole('heading', { name: 'Files' })).toBeVisible({ timeout: 5000 });
    const originalViewport = page.viewportSize();
    if (originalViewport) {
      await page.setViewportSize({
        width: originalViewport.width > 1000 ? originalViewport.width - 180 : originalViewport.width + 180,
        height: originalViewport.height > 640 ? originalViewport.height - 100 : originalViewport.height + 100,
      });
    }

    // Switch back to Terminal 1
    await page.click('.tab:has-text("Terminal")');
    await page.waitForSelector('.terminal-window', { timeout: 5000 });
    // Wait for xterm to re-fit after tab return
    const termFrameAfter = getTerminalFrame(page);
    if (!termFrameAfter) throw new Error('Terminal iframe disappeared after tab return');
    await waitForFit(termFrameAfter);

    const geometryBeforeDelayedFit = await termFrameAfter.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));
    const refitStartedAt = await termFrameAfter.evaluate(() => {
      const probeWindow = window as RefitProbeWindow;
      if (!probeWindow.__chroteRefitProbe) throw new Error('refit probe was not installed');
      probeWindow.__chroteRefitProbe.events = [];
      return performance.now();
    });
    await page.getByRole('button', { name: 'Refit terminal layout' }).click();
    await expect.poll(async () => termFrameAfter.evaluate(() =>
      (window as RefitProbeWindow).__chroteRefitProbe?.events.length ?? 0
    )).toBeGreaterThan(0);

    // Change geometry after the immediate refit but before the 200 ms recovery pass.
    await page.waitForTimeout(80);
    const revealedViewport = page.viewportSize();
    if (!revealedViewport) throw new Error('Playwright viewport is unavailable');
    await page.setViewportSize({
      width: revealedViewport.width > 1000 ? revealedViewport.width - 120 : revealedViewport.width + 120,
      height: revealedViewport.height > 640 ? revealedViewport.height - 60 : revealedViewport.height + 60,
    });
    await expect.poll(async () => termFrameAfter.evaluate(({ width, height }) =>
      window.innerWidth !== width || window.innerHeight !== height,
      geometryBeforeDelayedFit
    )).toBe(true);
    const finalGeometry = await termFrameAfter.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));
    await expect.poll(async () => termFrameAfter.evaluate(({ startedAt, geometry }) => {
      const events = (window as RefitProbeWindow).__chroteRefitProbe?.events ?? [];
      return events.some(event =>
        event.at - startedAt >= 180
        && event.width === geometry.width
        && event.height === geometry.height
      );
    }, { startedAt: refitStartedAt, geometry: finalGeometry }), {
      message: 'a delayed refit must observe the final post-reveal geometry',
      timeout: 1500,
    }).toBe(true);

    // After returning, iframe should still fill container
    const sizing = await page.evaluate(() => {
      const bodies = document.querySelectorAll('.terminal-window-body');
      for (const b of Array.from(bodies)) {
        const iframe = b.querySelector('iframe') as HTMLIFrameElement;
        if (!iframe || getComputedStyle(iframe).display === 'none') continue;
        const body = b as HTMLElement;
        return {
          containerWidth: body.clientWidth,
          containerHeight: body.clientHeight,
          iframeWidth: iframe.offsetWidth,
          iframeHeight: iframe.offsetHeight,
          fills: iframe.offsetWidth >= body.clientWidth - 2 && iframe.offsetHeight >= body.clientHeight - 2,
        };
      }
      return null;
    });

    console.log('Sizing after tab round-trip:', sizing);
    expect(sizing).not.toBeNull();
    expect(sizing!.fills).toBe(true);

    // Check xterm is not clipped and the marker cells are actually painted.
    const xtermSizing = await termFrameAfter.evaluate(marker => {
        const viewport = document.querySelector('.xterm-viewport') as HTMLElement;
        const screen = document.querySelector('.xterm-screen') as HTMLElement;
        const term = (window as typeof window & { term?: { cols: number; rows: number; buffer: { active: { cursorY: number; getLine: (row: number) => { translateToString: (trimRight?: boolean) => string } | undefined } } } }).term;
        const renderCanvas = Array.from(screen?.querySelectorAll('canvas') ?? [])
          .find(canvas => !canvas.classList.contains('xterm-link-layer'));
        if (!viewport || !screen || !term || !renderCanvas) return null;

        const inputLine = term.buffer.active.getLine(term.buffer.active.cursorY)?.translateToString(true) ?? '';
        const markerColumn = inputLine.indexOf(marker);
        const viewportRect = viewport.getBoundingClientRect();
        const canvasRect = renderCanvas.getBoundingClientRect();
        const cellWidthCss = canvasRect.width / term.cols;
        const cellHeightCss = canvasRect.height / term.rows;
        const markerRect = {
          left: canvasRect.left + markerColumn * cellWidthCss,
          right: canvasRect.left + (markerColumn + marker.length) * cellWidthCss,
          top: canvasRect.top + term.buffer.active.cursorY * cellHeightCss,
          bottom: canvasRect.top + (term.buffer.active.cursorY + 1) * cellHeightCss,
        };

        screen.querySelectorAll('[data-chrote-visual-probe]').forEach(node => node.remove());
        const blankColumn = markerColumn + marker.length + 4;
        const visualProbeReady = markerColumn >= 0 && blankColumn + marker.length <= term.cols;
        if (visualProbeReady) {
          const addProbe = (kind: 'marker' | 'blank', column: number) => {
            const probe = document.createElement('div');
            probe.dataset.chroteVisualProbe = kind;
            Object.assign(probe.style, {
              position: 'absolute',
              pointerEvents: 'none',
              left: `${column * cellWidthCss}px`,
              top: `${term.buffer.active.cursorY * cellHeightCss}px`,
              width: `${marker.length * cellWidthCss}px`,
              height: `${cellHeightCss}px`,
              zIndex: '20',
            });
            screen.appendChild(probe);
          };
          addProbe('marker', markerColumn);
          addProbe('blank', blankColumn);
        }

        return {
          isClipped: screen.scrollHeight > viewport.clientHeight + 5,
          cursorY: term.buffer.active.cursorY,
          rows: term.rows,
          inputLine,
          markerFullyVisible: markerColumn >= 0
            && markerRect.left >= viewportRect.left
            && markerRect.right <= viewportRect.right
            && markerRect.top >= viewportRect.top
            && markerRect.bottom <= viewportRect.bottom,
          visualProbeReady,
        };
    }, promptMarker);

    console.log('xterm sizing after tab switch:', xtermSizing);
    expect(xtermSizing).not.toBeNull();
    expect(xtermSizing!.isClipped).toBe(false);
    expect(xtermSizing!.cursorY).toBeLessThan(xtermSizing!.rows);
    expect(xtermSizing!.inputLine).toContain(promptMarker);
    expect(xtermSizing!.markerFullyVisible).toBe(true);
    expect(xtermSizing!.visualProbeReady).toBe(true);

    const markerPixels = await termFrameAfter.locator('[data-chrote-visual-probe="marker"]').screenshot();
    const blankPixels = await termFrameAfter.locator('[data-chrote-visual-probe="blank"]').screenshot();
    expect(markerPixels.equals(blankPixels), 'marker cells must contain painted glyphs unlike blank cells on the same row').toBe(false);
    await termFrameAfter.evaluate(() => {
      document.querySelectorAll('[data-chrote-visual-probe]').forEach(node => node.remove());
    });
  });
});
