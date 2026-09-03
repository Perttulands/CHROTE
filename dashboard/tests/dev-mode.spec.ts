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

async function openWorkspace(page: Page) {
  await page.setViewportSize({ width: 1400, height: 900 })
  await mockApiRoutes(page)
  await page.goto('/')
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
  await page.reload()
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
  await page.keyboard.type('dev')
  await expect(panel.locator('.keys-panel-chord')).toHaveCount(1)
  await page.keyboard.press('Enter')
  await expect(panel).toBeHidden()
}

test('dev mode names what the pointer is over and hands it to an agent', async ({ page }) => {
  await openWorkspace(page)

  await toggleDevMode(page)
  await expect(page.locator('.status-line')).toContainText('Dev mode on')

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
  await expect(page.locator('.status-line')).toContainText('Dev mode off')
})
