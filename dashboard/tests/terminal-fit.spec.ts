import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Delayed-readiness fit regression (beads: chrote-qlx, chrote-g1r).
 *
 * The real ttyd 1.7.7 client exposes window.term (and term.fit) from xterm
 * open() onward, but attaches its window resize listener only in
 * onSocketOpen — later. The old dashboard fit path dispatched synthetic
 * resize events in timed bursts (load+0/200/500ms), all of which land before
 * a slow socket open and vanish; the font retry loop then changed cell
 * metrics without ever fitting. Net effect: the grid kept stale geometry and
 * the TUI input row could sit below the visible iframe until manual Refit.
 *
 * This stub reproduces those timing semantics deterministically: term
 * appears 700ms after document start (after every legacy burst has fired),
 * the resize listener 400ms later still. The test passes only if the
 * dashboard fits WITHOUT any Refit click and only after the configured font
 * size (14) is in place — i.e. the closed-loop font-then-fit path.
 * Evidence for the timings: /srv/data/chrote/evidence/fit-probe-20260803/.
 */

const DELAYED_TTYD_STUB = `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<div class="xterm"><div class="xterm-viewport"><div class="xterm-screen">mock terminal</div></div></div>
<script>
  window.__fitLog = [];
  setTimeout(function () {
    var term = {
      options: { fontSize: 15 },
      fit: function () { window.__fitLog.push({ fontSize: term.options.fontSize }); },
    };
    window.term = term;
    setTimeout(function () {
      window.addEventListener('resize', function () { term.fit(); });
    }, 400);
  }, 700);
</script>
</body></html>`

const SEEDED_STATE = {
  workspaces: {
    terminal1: {
      windowCount: 1,
      windows: [
        { id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
      ],
    },
    terminal2: { windowCount: 1, windows: [] },
    terminal3: { windowCount: 1, windows: [] },
  },
  sidebarCollapsed: false,
  settings: {
    theme: 'dark',
    fontSize: 14,
    autoRefreshInterval: 1000,
  },
}

test.describe('Terminal fit under delayed ttyd readiness', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    // Registered after mockApiRoutes so it wins for /terminal documents.
    await page.route(/\/terminal(\/|\?|$)/, async route => {
      await route.fulfill({ status: 200, contentType: 'text/html', body: DELAYED_TTYD_STUB })
    })
    await page.addInitScript((state) => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, SEEDED_STATE)
  })

  test('fits automatically with the configured font once term appears, without manual Refit', async ({ page }) => {
    await page.goto('/')

    const frameElement = page.locator('.terminal-grid[data-workspace="terminal1"] iframe')
    await expect(frameElement).toBeVisible()
    await expect
      .poll(async () => frameElement.evaluate((el: HTMLIFrameElement) => el.src))
      .toContain('/terminal/?arg=main')

    // No Refit click, no viewport resize: the only path to a recorded fit is
    // the dashboard reaching term.fit() itself after term materializes.
    const fitFontSizes = await expect
      .poll(async () => frameElement.evaluate((el: HTMLIFrameElement) => {
        const win = el.contentWindow as (Window & { __fitLog?: { fontSize: number }[] }) | null
        return win?.__fitLog?.map(entry => entry.fontSize) ?? []
      }), { timeout: 5000 })
      .toEqual(expect.arrayContaining([14]))
      .then(() => frameElement.evaluate((el: HTMLIFrameElement) => {
        const win = el.contentWindow as (Window & { __fitLog?: { fontSize: number }[] }) | null
        return win?.__fitLog?.map(entry => entry.fontSize) ?? []
      }))

    // Font-then-fit: the very first fit must already see the configured font
    // size, never the ttyd default (15) — fitting before the font lands is
    // the stale-cell-metrics bug this spec pins.
    expect(fitFontSizes[0]).toBe(14)
  })
})
