import { expect, test } from '@playwright/test'

test('built server serves embedded assets and persists a safe Formations edit', async ({ page, request }) => {
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
})
