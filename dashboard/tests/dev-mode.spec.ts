import { expect, test, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { setWorkspaceState } from './helpers'

/**
 * Dev mode (bead: chrote-5grx.19).
 *
 * One journey: the operator turns dev mode on from the keys panel, points at a
 * tile header and reads what it is, clicks it, and finds the Send drawer open
 * with a line an agent can act on and a new CHROTE agent offered as a target.
 *
 * This is the browser's half of the proof that the component name is readable
 * off a rendered element at all — the fiber walk, real pointer geometry, and
 * the capture-phase click that must not press the button under it. That the
 * name also survives minification is proven where the minifier can be run:
 * `src/dev/keepNames.test.ts`.
 */

const TARGET = 'gt-gastown-jack'

const LEADER = 'Control+Shift+Space'

async function openWorkspace(page: Page, overrides: (page: Page) => Promise<void>) {
  await page.setViewportSize({ width: 1400, height: 900 })
  await mockApiRoutes(page, { overrides })
  await setWorkspaceState(page, {
    workspaces: {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: [TARGET], activeSession: TARGET, colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
        ],
      },
      terminal2: { windowCount: 2, windows: [] },
    },
  })
  await page.goto('/')
  await expect(page.locator('.terminal-window:visible')).toHaveCount(2)
}

/**
 * The chord has no Alt form — Chrome owns Alt+D — so it is run the way every
 * chord without one is run: the leader opens the keys panel, the search finds
 * the row, Enter runs it.
 */
async function toggleDevMode(page: Page) {
  await page.keyboard.press(LEADER)
  const panel = page.locator('.keys-panel')
  await expect(panel).toBeVisible()
  // The field takes the focus an effect after the panel paints; typing before
  // that drops the first letters and leaves the wrong row under the cursor.
  const search = panel.getByRole('textbox', { name: 'Search keybindings' })
  await expect(search).toBeFocused()
  await page.keyboard.type('dev')
  await expect(search).toHaveValue('dev')
  await expect(panel.locator('.keys-panel-chord')).toHaveCount(1)
  await page.keyboard.press('Enter')
  await expect(panel).toBeHidden()
}

async function expectBackgroundContent(page: Page) {
  // These hidden views render their loaded data in the same update that emits
  // their announcement. Both Beads stores must finish before that update.
  await expect(page.locator('.agents-view .agent-path')).toHaveText(/\/CLAUDE\.md$/)
  await expect(page.locator('.library-shelf-name')).toHaveText(['knowledge', 'preferences'])
  await expect(page.locator('.beads-view .bead-row', { hasText: 'One interaction language' })).toHaveCount(1)
  await expect(page.locator('.beads-view .bead-row', { hasText: 'Prepare loose work' })).toHaveCount(1)
}

for (const completion of ['before enabling', 'while enabled'] as const) {
  test(`dev mode identifies and hands off with background reads completing ${completion}`, async ({ page }) => {
    let release!: () => void
    const held = new Promise<void>(resolve => { release = resolve })
    const backgroundReads = ['**/api/agent/context**', '**/api/beads/work**', '**/api/library/shelves**']
    await openWorkspace(page, async page => {
      for (const pattern of backgroundReads) {
        await page.route(pattern, async route => {
          await held
          await route.fallback()
        })
      }
    })

    if (completion === 'before enabling') {
      release()
      await expectBackgroundContent(page)
    }
    await toggleDevMode(page)

    // Pointing at a tile header names the component that renders it and the file
    // it is written in. Nothing but the build's kept names can answer the first
    // half of that line.
    const header = page.locator('.terminal-window:visible').first().locator('.terminal-window-header')
    const send = header.getByRole('button', { name: `Send to session ${TARGET}` })
    await send.hover()

    // The outline snaps out to the nearest named surface, so the label reads the
    // header rather than whichever div the pointer happened to land in.
    const label = page.locator('.dev-mode-label')
    await expect(label).toHaveText('TerminalWindow · dashboard/src/components/TerminalWindow.tsx · tile.header')
    await expect(page.locator('.dev-mode-outline')).toBeVisible()

    if (completion === 'while enabled') {
      release()
      await expectBackgroundContent(page)
    }

    // The tag is its own component and says so.
    await header.locator('.session-tag').first().hover()
    await expect(label).toHaveText('SessionTag · dashboard/src/components/TerminalWindow.tsx · tile.tag')

    // The click annotates rather than pressing: the tile's Send button is under
    // the pointer and stays unpressed, and the drawer that opens is dev mode's.
    await send.click()

    const drawer = page.getByRole('dialog', { name: 'Send to session' })
    await expect(drawer).toBeVisible()
    await expect(drawer.locator('.send-drawer-reference')).toHaveText(
      `component TerminalWindow (dashboard/src/components/TerminalWindow.tsx) tile.header ` +
      `in terminal1 window 1: button 'Send to session ${TARGET}'`,
    )
    await expect(drawer.getByLabel('Message to send')).toBeFocused()

    // The complaint is CHROTE's own, so the picker offers a fresh agent for it.
    await expect(drawer.getByRole('option', { name: 'New agent in CHROTE' })).toBeVisible()

    // Dev mode ended with the annotation, so the drawer is usable again.
    await expect(page.locator('.dev-mode-label')).toHaveCount(0)
    const message = drawer.getByLabel('Message to send')
    await message.fill('The header needs attention')
    await expect(message).toHaveValue('The header needs attention')
  })
}
