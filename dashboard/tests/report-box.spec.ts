import { expect, test, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { setWorkspaceState } from './helpers'

/**
 * The complaint box (chrote-iz72): Alt+R over a focused terminal opens it, Enter
 * files a bug in CHROTE's store with the context line, and Ctrl+Enter files and
 * hands the new Bead to the Send drawer, whose Enter delivers it. The filing
 * route is mocked, so no store is written. The box's own choices, its draft
 * and its failure are unit tests beside the component.
 */

interface Filing {
  path: string
  title: string
  description: string
  type: string
}

const STORES = {
  success: true,
  data: {
    projects: [
      { name: 'test-project', path: '/code/test-project', beadsPath: '/code/test-project/.beads', prefix: 'test' },
      { name: 'chrote', path: '/code/chrote', beadsPath: '/code/chrote/.beads', prefix: 'chrote' },
    ],
  },
}

/** The field of a multipart body the browser posted to the send route. */
function field(body: string, name: string): string {
  const match = new RegExp(`name="${name}"\\r?\\n\\r?\\n([\\s\\S]*?)\\r?\\n--`).exec(body)
  return match ? match[1] : ''
}

async function mockRoutes(page: Page, filings: Filing[], sends: string[]) {
  await mockApiRoutes(page, {
    overrides: async page => {
      await page.route('**/api/beads/projects**', route => route.fulfill({ json: STORES }))
      await page.route('**/api/beads/issues', async route => {
        const body = route.request().postDataJSON() as Filing
        filings.push(body)
        await route.fulfill({ json: { success: true, data: { id: `chrote-new${filings.length}`, title: body.title } } })
      })
      await page.route('**/api/tmux/sessions/*/panes', route => route.fulfill({
        json: {
          success: true,
          session: 'main',
          unixUser: '',
          panes: [{
            sessionId: '$1', pane: '%1', panePid: '4242', serverPid: '9001', windowId: '@1',
            windowName: 'main', currentPath: '/srv/chrote', currentCommand: 'bash', active: true,
          }],
        },
      }))
      await page.route('**/api/tmux/sessions/*/send', async route => {
        const body = route.request().postData() ?? ''
        const submit = field(body, 'submit') === 'true'
        sends.push(field(body, 'text'))
        await route.fulfill({
          json: {
            success: true, session: 'main', sessionId: '$1', pane: '%1', panePid: '4242', serverPid: '9001',
            unixUser: '', transport: 'pasted', submissionRequested: submit, submitKeyDispatched: submit,
            bufferCleaned: true, targetVerified: true, deliveryConfirmed: true, retryable: false, warning: '',
          },
        })
      })
    },
  })
}

test('Alt+R files a complaint from a focused terminal and File & send hands it to the tile', async ({ page }) => {
  const filings: Filing[] = []
  const sends: string[] = []
  await page.setViewportSize({ width: 1400, height: 900 })
  await mockRoutes(page, filings, sends)
  await setWorkspaceState(page, {
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [{ id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 }],
      },
    },
  })
  await page.goto('/')

  const tile = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window').first()
  const terminal = tile.locator('.xterm-helper-textarea')
  await tile.locator('.xterm-screen').click()
  await expect(tile).toHaveClass(/focused/)

  // Filing: the first line is the title, the rest the description, and the
  // context line says where the operator was.
  await page.keyboard.press('Alt+r')
  const box = page.getByRole('dialog', { name: 'Report an issue' })
  const issue = box.getByRole('textbox', { name: 'Issue' })
  await expect(issue).toBeFocused()
  await expect(box.getByRole('combobox', { name: 'Store' })).toHaveValue('chrote')
  await issue.pressSequentially('Title')
  await issue.press('Shift+Enter')
  await issue.pressSequentially('more')
  await issue.press('Enter')

  await expect(box).toHaveCount(0)
  expect(filings).toEqual([{
    path: '/code/chrote',
    title: 'Title',
    description: 'more\n\nContext: terminal1 · main',
    type: 'bug',
  }])
  await expect(page.getByRole('status', { name: 'Status' })).toContainText('Filed chrote-new1')
  await expect(terminal).toBeFocused()

  // File & send: the drawer opens on the focused tile with the Bead named and
  // the note written, and Enter delivers both.
  await page.keyboard.press('Alt+r')
  await expect(issue).toHaveValue('')
  await issue.pressSequentially('Hand it over')
  await issue.press('Control+Enter')

  const drawer = page.getByRole('dialog', { name: 'Send to session' })
  await expect(drawer).toBeVisible()
  await expect(box).toHaveCount(0)
  await expect(drawer.getByRole('option', { name: 'main', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(drawer.getByText('bead chrote-new2: Hand it over')).toBeVisible()
  const note = drawer.getByLabel('Message to send')
  await expect(note).toHaveValue(/^I just filed this Bead\./)
  await expect(note).toBeFocused()
  await note.press('Enter')

  await expect(drawer).toHaveCount(0)
  expect(sends).toHaveLength(1)
  expect(sends[0]).toMatch(/^bead chrote-new2: Hand it over\r\n\r\nI just filed this Bead\./)
  // The box was gone before the drawer opened, so the drawer gives the focus
  // back to the terminal the complaint was made from.
  await expect(terminal).toBeFocused()
})
