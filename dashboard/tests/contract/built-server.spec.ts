import { expect, test } from '@playwright/test'

const contractSession = 'chrote-owc-contract'

test('built server serves embedded assets and persists a safe Formations edit', async ({ page, request }) => {
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

  const listResponse = await request.get('/api/formations/boards')
  expect(listResponse.ok(), `board list returned ${listResponse.status()}`).toBeTruthy()
  const listBody = await listResponse.json()
  expect(listBody.data.boards).toContainEqual(expect.objectContaining({
    slug: 'ci-contract',
    title: 'CI Contract Board',
  }))

  const boardResponse = await request.get('/api/formations/boards/ci-contract')
  expect(boardResponse.ok(), `board read returned ${boardResponse.status()}`).toBeTruthy()
  const boardBody = await boardResponse.json()
  const board = boardBody.data.board
  const etag = boardResponse.headers().etag
  expect(etag).toBeTruthy()

  const patchResponse = await request.patch('/api/formations/boards/ci-contract', {
    headers: { 'If-Match': etag },
    data: {
      createFormation: {
        type: 'solo',
        title: 'CI Contract Formation',
        x: 420,
        y: 180,
      },
      expectedRev: board.rev,
      updatedBy: 'agent:ci-contract',
    },
  })
  const patchBody = await patchResponse.json()
  expect(patchResponse.ok(), `formation create returned ${patchResponse.status()}: ${JSON.stringify(patchBody)}`).toBeTruthy()
  expect(patchBody.data.formation).toEqual(expect.objectContaining({
    type: 'solo',
    title: 'CI Contract Formation',
  }))

  await page.goto('/')
  await page.getByRole('button', { name: 'Formations' }).click()

  await expect(page.getByTestId('formations-view')).toBeVisible()
  await expect(page.getByTestId(`formation-node-${patchBody.data.formation.id}`))
    .toContainText('CI Contract Formation')

  await page.getByRole('button', { name: 'Terminal', exact: true }).click()
  const terminal1Dock = page.locator('.terminal-workspace-dock[data-workspace="terminal1"][data-active="true"]')
  await terminal1Dock.getByRole('button', { name: 'Sessions sidecar' }).click()
  const sessionItem = terminal1Dock.locator('.session-item').filter({ hasText: contractSession })
  await expect(sessionItem).toBeVisible()

  await sessionItem.getByRole('button', { name: `Session actions for ${contractSession}` }).click()
  await page.getByRole('button', { name: 'Attach to Window' }).click()
  const terminalResponsePromise = page.waitForResponse(response => (
    response.request().resourceType() === 'document' &&
    response.url().includes('/terminal/')
  ))
  await page.getByRole('button', { name: 'Window 1', exact: true }).click()
  const terminalResponse = await terminalResponsePromise
  expect(terminalResponse.ok(), `terminal route returned ${terminalResponse.status()}`).toBeTruthy()
  expect(terminalResponse.url()).toContain('/terminal/')
  expect(await terminalResponse.text()).toContain('CHROTE_OWC_TTYD_SENTINEL')
  await expect(page.locator(`iframe[title="Terminal - ${contractSession}"]`)).toHaveAttribute('src', /\/terminal\//)
  await expect(page.frameLocator(`iframe[title="Terminal - ${contractSession}"]`).locator('body'))
    .toContainText('CHROTE_OWC_TTYD_SENTINEL')

  for (const contractPath of [contractFilesTerminal1, contractFilesTerminal2]) {
    const filesResponse = await request.get(`/api/files/resources${contractPath}`)
    const filesBody = await filesResponse.json()
    expect(filesResponse.ok(), `Files route returned ${filesResponse.status()} for ${contractPath}: ${JSON.stringify(filesBody)}`).toBeTruthy()
    expect(filesBody).toEqual(expect.objectContaining({ isDir: true }))
  }

  const filesButton1 = terminal1Dock.getByRole('button', { name: 'Files sidecar' })
  await filesButton1.click()
  const filesPanel1 = terminal1Dock.locator('[aria-label="Files sidecar"]')
  const filesPath1 = filesPanel1.getByRole('textbox', { name: 'Files panel path' })
  const filesResponsePromise1 = page.waitForResponse(response => (
    response.request().method() === 'GET' &&
    response.request().resourceType() === 'fetch' &&
    response.url().includes('/api/files/resources') &&
    response.url().includes('contract-files-terminal1')
  ))
  await filesPath1.fill(contractFilesTerminal1)
  await filesPath1.press('Enter')
  const filesResponse1 = await filesResponsePromise1
  expect(filesResponse1.ok(), `Files sidecar route returned ${filesResponse1.status()}`).toBeTruthy()
  await expect(filesPath1).toHaveValue(contractFilesTerminal1)

  await page.getByRole('button', { name: 'Terminal 2', exact: true }).click()
  const terminal2Dock = page.locator('.terminal-workspace-dock[data-workspace="terminal2"][data-active="true"]')
  await expect(terminal2Dock.locator('.session-panel')).toHaveAttribute('data-active-workspace', 'terminal2')
  await expect(terminal2Dock.locator('.session-item').filter({ hasText: contractSession })).toBeVisible()

  await terminal2Dock.getByRole('button', { name: 'Files sidecar' }).click()
  const filesPanel2 = terminal2Dock.locator('[aria-label="Files sidecar"]')
  const filesPath2 = filesPanel2.getByRole('textbox', { name: 'Files panel path' })
  await expect(filesPath2).toHaveValue('/')
  const filesResponsePromise2 = page.waitForResponse(response => (
    response.request().method() === 'GET' &&
    response.request().resourceType() === 'fetch' &&
    response.url().includes('/api/files/resources') &&
    response.url().includes('contract-files-terminal2')
  ))
  await filesPath2.fill(contractFilesTerminal2)
  await filesPath2.press('Enter')
  const filesResponse2 = await filesResponsePromise2
  expect(filesResponse2.ok(), `Files sidecar route returned ${filesResponse2.status()}`).toBeTruthy()
  await expect(filesPath2).toHaveValue(contractFilesTerminal2)

  await page.getByRole('button', { name: 'Terminal', exact: true }).click()
  await expect(terminal1Dock.locator('[aria-label="Files sidecar"]')
    .getByRole('textbox', { name: 'Files panel path' })).toHaveValue(contractFilesTerminal1)
  await page.getByRole('button', { name: 'Terminal 2', exact: true }).click()
  await expect(filesPanel2.getByRole('textbox', { name: 'Files panel path' })).toHaveValue(contractFilesTerminal2)
})
