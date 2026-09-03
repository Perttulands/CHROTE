import { expect, test } from '@playwright/test'

const contractSession = 'chrote-owc-contract'

test('built server serves embedded assets and preserves terminal and Files workflows', async ({ page, request }) => {
  const contractWorkspace = process.env.CHROTE_CONTRACT_WORKSPACE
  expect(contractWorkspace, 'contract workspace must be task-owned and supplied by the wrapper').toBeTruthy()
  const contractFilesTerminal1 = `${contractWorkspace}/contract-files-terminal1`
  const contractFilesTerminal2 = `${contractWorkspace}/contract-files-terminal2`

  const indexResponse = await request.get('/')
  expect(indexResponse.ok(), `embedded dashboard returned ${indexResponse.status()}`).toBeTruthy()
  expect(indexResponse.headers()['content-type']).toContain('text/html')

  const indexHTML = await indexResponse.text()
  const scriptPath = indexHTML.match(/<script[^>]+src="([^"]+\.js)"/)?.[1]
  expect(scriptPath, 'embedded dashboard index should reference a JavaScript asset').toBeTruthy()

  const scriptResponse = await request.get(scriptPath!)
  expect(scriptResponse.ok(), `embedded asset ${scriptPath} returned ${scriptResponse.status()}`).toBeTruthy()
  expect(scriptResponse.headers()['content-type']).toMatch(/javascript/)
  expect((await scriptResponse.body()).byteLength).toBeGreaterThan(1000)

  await page.goto('/')
  await page.getByRole('button', { name: 'Terminal', exact: true }).click()
  const terminal1Dock = page.locator('.terminal-workspace-dock[data-workspace="terminal1"][data-active="true"]')
  await terminal1Dock.getByRole('button', { name: 'Sessions sidecar' }).click()
  const sessionItem = terminal1Dock.locator('.session-item').filter({ hasText: contractSession })
  await expect(sessionItem).toBeVisible()

  await sessionItem.getByRole('button', { name: `Session actions for ${contractSession}` }).click()
  await page.getByRole('menuitem', { name: 'Attach to window' }).click()
  await page.getByRole('menuitem', { name: 'Window 1', exact: true }).click()

  // The dashboard renders the terminal itself, in this document (ADR-0018).
  await expect(terminal1Dock.locator('.terminal-window-body .terminal-surface')).toBeVisible()

  // CHROTE serves the terminal itself (ADR-0018): nothing under /terminal
  // answers a plain HTTP request, and there is no ttyd page, asset or /token
  // behind it any more.
  for (const removedPath of ['/terminal/', '/terminal/token', '/terminal/xterm.css', '/terminal/ws']) {
    const removed = await request.get(removedPath)
    expect(removed.status(), `${removedPath} should not be served over plain HTTP`).toBe(404)
  }

  // The upgrade route is still wired: this request asks for one without a
  // WebSocket key, so it is refused by the upgrade rather than by the route.
  const upgrade = await request.get(`/terminal/ws?arg=tile&arg=${contractSession}`, {
    headers: { Upgrade: 'websocket', Connection: 'Upgrade' },
  })
  expect(upgrade.status(), 'the terminal WebSocket route must still be served').not.toBe(404)

  for (const contractPath of [contractFilesTerminal1, contractFilesTerminal2]) {
    const filesResponse = await request.get(`/api/files/resources${contractPath}`)
    const filesBody = await filesResponse.json()
    expect(filesResponse.ok(), `Files route returned ${filesResponse.status()} for ${contractPath}: ${JSON.stringify(filesBody)}`).toBeTruthy()
    expect(filesBody).toEqual(expect.objectContaining({ isDir: true }))
  }

  // The panel is search-first, so the contract proves the routes it now leans
  // on: the write route that the in-panel editor saves through, the find route
  // that reaches a file by name, and the per-workspace open file.
  const contractFile1 = `${contractFilesTerminal1}/contract-find-terminal1.md`
  const contractFile2 = `${contractFilesTerminal2}/contract-find-terminal2.md`
  for (const [contractFile, heading] of [[contractFile1, 'Terminal 1 contract'], [contractFile2, 'Terminal 2 contract']] as const) {
    const written = await request.post(`/api/files/resources${contractFile}`, {
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
      data: `# ${heading}\n`,
    })
    expect(written.ok(), `write route returned ${written.status()} for ${contractFile}`).toBeTruthy()
  }

  const diffResponse = await request.get(`/api/files/diff?path=${encodeURIComponent(contractFile1)}`)
  expect(diffResponse.ok(), `diff route returned ${diffResponse.status()}`).toBeTruthy()
  expect(await diffResponse.json()).toEqual(expect.objectContaining({ repository: '', diff: '' }))

  const filesButton1 = terminal1Dock.getByRole('button', { name: 'Files sidecar' })
  await filesButton1.click()
  const filesPanel1 = terminal1Dock.locator('[aria-label="Files sidecar"]')
  const findField1 = filesPanel1.getByRole('textbox', { name: 'Find files' })
  const findResponsePromise1 = page.waitForResponse(response => (
    response.request().method() === 'GET' &&
    response.url().includes('/api/files/find') &&
    response.url().includes('contract-find-terminal1')
  ))
  await findField1.fill('contract-find-terminal1')
  const findResponse1 = await findResponsePromise1
  expect(findResponse1.ok(), `find route returned ${findResponse1.status()}`).toBeTruthy()
  await filesPanel1.getByRole('listitem').filter({ hasText: 'contract-find-terminal1.md' }).first().click()
  await expect(filesPanel1.getByRole('heading', { name: 'Terminal 1 contract' })).toBeVisible()

  await page.getByRole('button', { name: 'Terminal 2', exact: true }).click()
  const terminal2Dock = page.locator('.terminal-workspace-dock[data-workspace="terminal2"][data-active="true"]')
  await expect(terminal2Dock.locator('.session-panel')).toHaveAttribute('data-active-workspace', 'terminal2')
  await expect(terminal2Dock.locator('.session-item').filter({ hasText: contractSession })).toBeVisible()

  await terminal2Dock.getByRole('button', { name: 'Files sidecar' }).click()
  const filesPanel2 = terminal2Dock.locator('[aria-label="Files sidecar"]')
  const findField2 = filesPanel2.getByRole('textbox', { name: 'Find files' })
  await expect(findField2).toHaveValue('')
  const findResponsePromise2 = page.waitForResponse(response => (
    response.request().method() === 'GET' &&
    response.url().includes('/api/files/find') &&
    response.url().includes('contract-find-terminal2')
  ))
  await findField2.fill('contract-find-terminal2')
  const findResponse2 = await findResponsePromise2
  expect(findResponse2.ok(), `find route returned ${findResponse2.status()}`).toBeTruthy()
  await filesPanel2.getByRole('listitem').filter({ hasText: 'contract-find-terminal2.md' }).first().click()
  await expect(filesPanel2.getByRole('heading', { name: 'Terminal 2 contract' })).toBeVisible()

  // Each workspace keeps its own open file across a tab switch.
  await page.getByRole('button', { name: 'Terminal', exact: true }).click()
  await expect(terminal1Dock.locator('[aria-label="Files sidecar"]')
    .getByRole('heading', { name: 'Terminal 1 contract' })).toBeVisible()
  await page.getByRole('button', { name: 'Terminal 2', exact: true }).click()
  await expect(filesPanel2.getByRole('heading', { name: 'Terminal 2 contract' })).toBeVisible()
})
