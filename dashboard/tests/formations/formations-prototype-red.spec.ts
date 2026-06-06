import { expect, Page, Route, test } from '../fixtures'
import { mockApiRoutes } from '../mock-api'

type Slot = {
  id: string
  label: string
  controller: boolean
  agentId?: string
  harness?: string
}

type Formation = {
  id: string
  type: 'solo' | 'peer' | 'flow' | 'orchestrated'
  title: string
  brief?: { goal?: string; beadId?: string; files?: string[]; links?: string[] }
  inputs: { id: string; label: string }[]
  outputs: { id: string; label: string }[]
  slots: Slot[]
}

type Gate = {
  id: string
  title: string
  kinds: string[]
  criterion: string
}

type Connection = {
  id: string
  from: string
  to: string
}

type Board = {
  id: string
  slug: string
  title: string
  rev: number
  etag: string
  missions: { id: string; title: string; goal: string; beadId: string }[]
  formations: Formation[]
  gates: Gate[]
  connections: Connection[]
}

type Layout = {
  boardId: string
  boardRev: number
  etag: string
  nodes: { id: string; x: number; y: number }[]
  edges: { id: string; lane: string }[]
}

type EndpointPatch = {
  from: string
  to: string
}

type GateJudgePatch = {
  gateId: string
  chain?: string[]
}

type BoardPatch = {
  wireConnection?: EndpointPatch
  unwireConnection?: EndpointPatch
  rewireConnection?: EndpointPatch & { previousTo: string }
  setGateJudge?: GateJudgePatch
  detachGateJudge?: { gateId: string }
  createFormation?: { id?: string; type?: Formation['type']; title?: string; x?: number; y?: number }
  assignSlot?: { formationId: string; slotId: string; agentId?: string; harness?: string }
  setBrief?: { formationId: string; goal?: string; beadId?: string; files?: string[]; links?: string[] }
  clearBrief?: { formationId: string }
}

type LayoutPatch = {
  nodes?: { id: string; x: number; y: number }[]
  edges?: { id: string; lane: string }[]
}

function baseBoard(): Board {
  return {
    id: 'board-prototype-red',
    slug: 'prototype-red',
    title: 'Prototype red board',
    rev: 3,
    etag: '"board-prototype-red-rev-3"',
    missions: [
      {
        id: 'mission-showcase',
        title: 'Showcase mission',
        goal: 'Build a direct-manipulation Formations flow',
        beadId: 'home-28ps.3',
      },
    ],
    formations: [
      {
        id: 'formation-frame',
        type: 'orchestrated',
        title: 'Frame',
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [{ id: 'out', label: 'output' }],
        slots: [
          {
            id: 'slot-frame-lead',
            label: 'Lead',
            controller: true,
            agentId: 'codex',
            harness: 'openai-codex',
          },
        ],
      },
      {
        id: 'formation-research',
        type: 'solo',
        title: 'Research',
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [{ id: 'out', label: 'output' }],
        slots: [
          {
            id: 'slot-research',
            label: 'Researcher',
            controller: false,
            agentId: 'ghost',
            harness: 'claude-code',
          },
        ],
      },
      {
        id: 'formation-ship',
        type: 'solo',
        title: 'Ship',
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [{ id: 'out', label: 'output' }],
        slots: [{ id: 'slot-ship', label: 'Shipper', controller: false }],
      },
      {
        id: 'formation-critique',
        type: 'peer',
        title: 'Critique',
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [{ id: 'out', label: 'output' }],
        slots: [
          { id: 'slot-critique-a', label: 'Peer A', controller: false },
          { id: 'slot-critique-b', label: 'Peer B', controller: false },
        ],
      },
      {
        id: 'formation-steps',
        type: 'flow',
        title: 'Steps',
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [{ id: 'out', label: 'output' }],
        slots: [
          { id: 'slot-step-1', label: 'Step 1', controller: false },
          { id: 'slot-step-2', label: 'Step 2', controller: false },
        ],
      },
    ],
    gates: [
      {
        id: 'gate-review',
        title: 'Review gate',
        kinds: ['human'],
        criterion: 'Decide whether the work is ready',
      },
    ],
    connections: [
      {
        id: 'edge-frame-research',
        from: 'formation-frame:out',
        to: 'formation-research:in',
      },
    ],
  }
}

function baseLayout(board: Board): Layout {
  return {
    boardId: board.id,
    boardRev: board.rev,
    etag: '"layout-prototype-red-rev-3"',
    nodes: [
      { id: 'mission-showcase', x: 60, y: 40 },
      { id: 'formation-frame', x: 300, y: 120 },
      { id: 'formation-research', x: 650, y: 120 },
      { id: 'formation-ship', x: 1000, y: 120 },
      { id: 'formation-critique', x: 300, y: 680 },
      { id: 'formation-steps', x: 650, y: 680 },
      { id: 'gate-review', x: 650, y: 380 },
    ],
    edges: [{ id: 'edge-frame-research', lane: 'auto' }],
  }
}

async function mountFormations(page: Page) {
  await page.addInitScript(() => {
    window.localStorage.setItem('chrote-formations', '1')
    // These specs pin the legacy form-style view; the rebuilt cockpit (default-on) is
    // covered by formations-cockpit.spec.ts with reference-grounded checks.
    window.localStorage.setItem('chrote-formations-cockpit', '0')
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Formations' }).click()
  await expect(page.getByTestId('formations-view')).toBeVisible()
}

async function dragBetween(page: Page, fromTestId: string, toTestId: string) {
  const from = page.getByTestId(fromTestId)
  const to = page.getByTestId(toTestId)
  await expect(from).toBeVisible({ timeout: 1500 })
  await expect(to).toBeVisible({ timeout: 1500 })
  const fromBox = await from.boundingBox()
  const toBox = await to.boundingBox()
  expect(fromBox, `${fromTestId} should have a box`).not.toBeNull()
  expect(toBox, `${toTestId} should have a box`).not.toBeNull()
  await from.dispatchEvent('pointerdown', pointerEventInit(fromBox!, 'down'))
  await to.dispatchEvent('pointerup', pointerEventInit(toBox!, 'up'))
  await to.dispatchEvent('mouseup', pointerEventInit(toBox!, 'up'))
}

async function dragHandle(page: Page, testId: string, dx: number, dy: number) {
  const handle = page.getByTestId(testId)
  await expect(handle).toBeVisible()
  const box = await handle.boundingBox()
  expect(box, `${testId} should have a box`).not.toBeNull()
  const start = { x: Math.round(box!.x + Math.min(20, box!.width / 2)), y: Math.round(box!.y + Math.min(12, box!.height / 2)) }
  await handle.dispatchEvent('pointerdown', {
    bubbles: true,
    cancelable: true,
    button: 0,
    buttons: 1,
    clientX: start.x,
    clientY: start.y,
  })
  await page.evaluate(({ x, y }) => {
    window.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, clientX: x, clientY: y, button: 0, buttons: 1 }))
    window.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, clientX: x, clientY: y, button: 0 }))
  }, { x: start.x + dx, y: start.y + dy })
}

async function dragToCanvasPoint(page: Page, testId: string, canvasPoint: { x: number; y: number }) {
  const handle = page.getByTestId(testId)
  const canvas = page.getByTestId('formations-canvas')
  await expect(handle).toBeVisible()
  await expect(canvas).toBeVisible()
  const handleBox = await handle.boundingBox()
  const canvasBox = await canvas.boundingBox()
  expect(handleBox, `${testId} should have a box`).not.toBeNull()
  expect(canvasBox, 'formations canvas should have a box').not.toBeNull()
  const start = { x: Math.round(handleBox!.x + handleBox!.width / 2), y: Math.round(handleBox!.y + handleBox!.height / 2) }
  const end = { x: Math.round(canvasBox!.x + canvasPoint.x), y: Math.round(canvasBox!.y + canvasPoint.y) }
  await handle.dispatchEvent('pointerdown', {
    bubbles: true,
    cancelable: true,
    button: 0,
    buttons: 1,
    clientX: start.x,
    clientY: start.y,
  })
  await page.evaluate(({ x, y }) => {
    window.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, clientX: x, clientY: y, button: 0, buttons: 1 }))
    window.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, clientX: x, clientY: y, button: 0 }))
  }, end)
}

function pointerEventInit(box: { x: number; y: number; width: number; height: number }, phase: 'down' | 'up') {
  return {
    bubbles: true,
    cancelable: true,
    button: 0,
    buttons: phase === 'down' ? 1 : 0,
    clientX: Math.round(box.x + box.width / 2),
    clientY: Math.round(box.y + box.height / 2),
  }
}

async function openContextMenu(page: Page, testId: string) {
  const target = page.getByTestId(testId)
  await expect(target).toBeVisible()
  const box = await target.boundingBox()
  expect(box, `${testId} should have a box`).not.toBeNull()
  await target.evaluate((element, point) => {
    element.dispatchEvent(new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      button: 2,
      buttons: 2,
      clientX: point.x,
      clientY: point.y,
    }))
  }, {
    x: Math.round(box!.x + box!.width / 2),
    y: Math.round(box!.y + box!.height / 2),
  })
}

async function clickAtCenter(page: Page, testId: string) {
  const target = page.getByTestId(testId)
  await expect(target).toBeVisible()
  const box = await target.boundingBox()
  expect(box, `${testId} should have a box`).not.toBeNull()
  await target.evaluate((element, point) => {
    element.dispatchEvent(new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
      button: 0,
      clientX: point.x,
      clientY: point.y,
    }))
  }, {
    x: Math.round(box!.x + box!.width / 2),
    y: Math.round(box!.y + box!.height / 2),
  })
}

async function installPrototypeHarness(page: Page, options?: { enableBoardChange?: boolean }) {
  let board = baseBoard()
  let layout = baseLayout(board)
  const boardPatches: BoardPatch[] = []
  const layoutPatches: LayoutPatch[] = []
  const tmuxMutations: string[] = []
  let changesServed = 0

  await mockApiRoutes(page)

  page.on('request', request => {
    const url = new URL(request.url())
    if (url.pathname.startsWith('/api/tmux/') && url.pathname !== '/api/tmux/appearance' && request.method() !== 'GET') {
      tmuxMutations.push(`${request.method()} ${url.pathname}`)
    }
  })

  await page.route('**/api/agents**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          agents: [
            {
              id: 'codex',
              displayName: 'Codex',
              harnessDefault: 'openai-codex',
              assignable: true,
              liveness: 'live',
            },
            {
              id: 'ghost',
              displayName: 'Ghost',
              harnessDefault: 'claude-code',
              assignable: true,
              liveness: 'dead',
            },
          ],
        },
      }),
    })
  })

  await page.route('**/api/formations/**', async (route: Route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname

    if (request.method() === 'GET' && path === '/api/formations/boards') {
      await json(route, {
        boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }],
      })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}`) {
      await json(route, { board }, { ETag: board.etag })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/layout`) {
      await json(route, { layout }, { ETag: layout.etag })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/changes`) {
      changesServed += 1
      if (options?.enableBoardChange && changesServed === 1) {
        board = withNextRev({
          ...board,
          title: 'Prototype red board reloaded',
          connections: [
            ...board.connections,
            { id: 'edge-external-ship', from: 'formation-research:out', to: 'formation-ship:in' },
          ],
        })
        layout = { ...layout, boardRev: board.rev, etag: `"layout-prototype-red-rev-${board.rev}"` }
        await json(route, {
          signal: { board: board.slug, changed: true, rev: board.rev, etag: board.etag },
        })
        return
      }
      await json(route, {
        signal: { board: board.slug, changed: false, rev: board.rev, etag: board.etag },
      })
      return
    }

    if (request.method() === 'PATCH' && path === `/api/formations/boards/${board.slug}`) {
      const body = request.postDataJSON() as BoardPatch
      const beforeFormationIds = new Set(board.formations.map(formation => formation.id))
      boardPatches.push(body)
      board = applyBoardPatch(board, body)
      const createdFormation = board.formations.find(formation => !beforeFormationIds.has(formation.id))
      layout = {
        ...layout,
        boardRev: board.rev,
        etag: `"layout-prototype-red-rev-${board.rev}"`,
        nodes: createdFormation
          ? [
            ...layout.nodes.filter(node => node.id !== createdFormation.id),
            {
              id: createdFormation.id,
              x: body.createFormation?.x ?? 120,
              y: body.createFormation?.y ?? 120,
            },
          ]
          : layout.nodes,
      }
      await json(route, { board, layout }, { ETag: board.etag })
      return
    }

    if (request.method() === 'PATCH' && path === `/api/formations/boards/${board.slug}/layout`) {
      const body = request.postDataJSON() as LayoutPatch
      layoutPatches.push(body)
      layout = applyLayoutPatch(layout, body)
      await json(route, { layout }, { ETag: layout.etag })
      return
    }

    if (request.method() === 'POST' && path === '/api/formations/runs') {
      await json(route, {
        runId: 'run-prototype-red',
        status: {
          runId: 'run-prototype-red',
          status: 'blocked',
          final: false,
          boardSlug: board.slug,
          missionId: 'mission-showcase',
          eventCount: 2,
          resumeAllowed: true,
        },
      })
      return
    }

    if (request.method() === 'GET' && path === '/api/formations/runs/run-prototype-red') {
      await json(route, {
        status: {
          runId: 'run-prototype-red',
          status: 'blocked',
          final: false,
          boardSlug: board.slug,
          missionId: 'mission-showcase',
          eventCount: 2,
          resumeAllowed: true,
        },
      })
      return
    }

    if (
      request.method() === 'GET' &&
      (path === '/api/formations/runs/run-prototype-red/events' || path === '/api/formations/runs/run-prototype-red/stream')
    ) {
      await route.fulfill({
        status: 200,
        contentType: path.endsWith('/stream') ? 'text/event-stream' : 'application/json',
        body: path.endsWith('/stream')
          ? 'id: 2\nevent: run_blocked\ndata: {"seq":2,"type":"run_blocked","runId":"run-prototype-red","gateId":"gate-review","data":{"reason":"waiting for human","resumeAllowed":true}}\n\n'
          : JSON.stringify({
            success: true,
            data: {
              events: [
                { seq: 1, type: 'run_started', runId: 'run-prototype-red', data: { actor: 'archon' } },
                { seq: 2, type: 'run_blocked', runId: 'run-prototype-red', gateId: 'gate-review', data: { reason: 'waiting for human', resumeAllowed: true } },
              ],
            },
          }),
      })
      return
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ success: false, error: { code: 'MOCK_NOT_FOUND', message: `${request.method()} ${path}` } }),
    })
  })

  return {
    boardPatches,
    layoutPatches,
    tmuxMutations,
    currentBoard: () => board,
    currentLayout: () => layout,
  }
}

test.describe('Formations D7 prototype RED coverage', () => {
  test('FORMATIONS.md + 03-formations.html: first viewport is a spatial cockpit with mission, typed cards, gate, and wire geometry', async ({ page }) => {
    await installPrototypeHarness(page)
    await mountFormations(page)

    await expect(page.getByTestId('formations-canvas')).toBeVisible()
    await expect(page.getByTestId('mission-node-mission-showcase')).toBeVisible()
    await expect(page.getByTestId('gate-node-gate-review')).toBeVisible()
    await expect(page.getByTestId('formation-wire-edge-frame-research')).toBeVisible()
    await expect(page.getByTestId('formation-node-formation-frame')).toHaveAttribute('data-formation-type', 'orchestrated')
    await expect(page.getByTestId('formation-node-formation-research')).toHaveAttribute('data-formation-type', 'solo')
    await expect(page.getByTestId('formation-node-formation-critique')).toHaveAttribute('data-formation-type', 'peer')
    await expect(page.getByTestId('formation-node-formation-steps')).toHaveAttribute('data-formation-type', 'flow')
    await expect(page.getByText('one controller')).toBeVisible()
    await expect(page.getByTestId('formation-node-formation-critique')).toContainText('peers, no hierarchy')
    await expect(page.getByTestId('formation-node-formation-steps')).toContainText('ordered steps')
  })

  test('03-formations.js canvas.feature: mission and gate nodes move, and mission output wires into the graph', async ({ page }) => {
    const harness = await installPrototypeHarness(page)
    await mountFormations(page)

    await dragHandle(page, 'mission-node-mission-showcase', 90, 36)
    await expect.poll(() => harness.currentLayout().nodes.find(node => node.id === 'mission-showcase')?.x).toBe(150)
    await expect.poll(() => harness.currentLayout().nodes.find(node => node.id === 'mission-showcase')?.y).toBe(76)

    await dragHandle(page, 'gate-node-gate-review', -70, 42)
    await expect.poll(() => harness.currentLayout().nodes.find(node => node.id === 'gate-review')?.x).toBe(580)
    await expect.poll(() => harness.currentLayout().nodes.find(node => node.id === 'gate-review')?.y).toBe(422)

    await dragBetween(page, 'mission-output-mission-showcase-out', 'formation-input-formation-frame-in')
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'mission-showcase:out' && connection.to === 'formation-frame:in'
    ))).toBe(true)
  })

  test('formations-and-slots + canvas undo: slot assignment and brief edits write through the board and undo outside text fields', async ({ page }) => {
    const harness = await installPrototypeHarness(page)
    await mountFormations(page)

    await page.getByLabel('Agent for Shipper').selectOption('codex|openai-codex')
    await expect.poll(() => harness.currentBoard().formations.find(formation => formation.id === 'formation-ship')?.slots[0]?.agentId).toBe('codex')

    await page.getByLabel('Goal for Ship').fill('draft while focused')
    const patchesBeforeFocusedUndo = harness.boardPatches.length
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
    expect(harness.boardPatches.length).toBe(patchesBeforeFocusedUndo)
    await page.getByTestId('formations-canvas').click({ position: { x: 16, y: 16 } })
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
    await expect.poll(() => harness.currentBoard().formations.find(formation => formation.id === 'formation-ship')?.slots[0]?.agentId || '').toBe('')

    await page.getByLabel('Goal for Ship').fill('Ship the verified flow')
    await page.getByLabel('Bead for Ship').fill('home-vdki.24')
    await page.getByLabel('Save brief for Ship').click()
    await expect.poll(() => harness.currentBoard().formations.find(formation => formation.id === 'formation-ship')?.brief?.goal).toBe('Ship the verified flow')
    await page.getByLabel('Goal for Ship').focus()
    const patchesBeforeFocusedBriefUndo = harness.boardPatches.length
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
    expect(harness.boardPatches.length).toBe(patchesBeforeFocusedBriefUndo)
    await page.getByTestId('formations-canvas').click({ position: { x: 16, y: 16 } })
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
    await expect.poll(() => harness.currentBoard().formations.find(formation => formation.id === 'formation-ship')?.brief?.goal || '').toBe('')
  })

  test('03-formations.js openTerm + terminals.feature: on-canvas terminals open, focus, stream, and close without session mutation', async ({ page }) => {
    const harness = await installPrototypeHarness(page)
    await mountFormations(page)

    await openContextMenu(page, 'formation-slot-slot-frame-lead')
    const openTerminal = page.getByRole('menuitem', { name: 'Open terminal' })
    await expect(openTerminal).toBeEnabled()
    await openTerminal.click()

    const terminal = page.getByTestId('formation-terminal-codex')
    await expect(terminal).toBeVisible()
    await expect(terminal).toContainText('Codex')
    await expect(terminal).toContainText('live')
    await expect(terminal.frameLocator('iframe').locator('body')).toContainText('mock terminal')

    await openContextMenu(page, 'formation-slot-slot-frame-lead')
    await page.getByRole('menuitem', { name: 'Open terminal' }).click()
    await expect(page.getByTestId('formation-terminal-codex')).toHaveCount(1)

    await page.getByRole('button', { name: 'Close terminal Codex' }).dispatchEvent('click')
    await expect(page.getByTestId('formation-terminal-codex')).toHaveCount(0)
    expect(harness.tmuxMutations).toEqual([])
  })

  test('terminals.feature + canvas.feature: terminal popups live in world space and expose dead-session state', async ({ page }) => {
    await installPrototypeHarness(page)
    await mountFormations(page)

    await openContextMenu(page, 'formation-slot-slot-frame-lead')
    const openTerminal = page.getByRole('menuitem', { name: 'Open terminal' })
    await expect(openTerminal).toBeEnabled()
    await openTerminal.click()
    const terminal = page.getByTestId('formation-terminal-codex')
    const firstBox = await terminal.boundingBox()
    await page.getByLabel('Zoom in').click()
    await expect(terminal).toHaveAttribute('data-world-scale', '1.2')
    await page.mouse.move(120, 500)
    await page.mouse.down()
    await page.mouse.move(180, 520)
    await page.mouse.up()
    const pannedBox = await terminal.boundingBox()
    expect(firstBox).not.toBeNull()
    expect(pannedBox).not.toBeNull()
    expect(pannedBox!.x).not.toBe(firstBox!.x)

    await dragHandle(page, 'formation-terminal-codex-header', 60, 38)
    await expect(terminal).toHaveAttribute('data-dragged', 'true')

    await dragHandle(page, 'formation-terminal-codex-resize', 66, 41)
    await expect(terminal).toHaveAttribute('data-resized', 'true')

    await openContextMenu(page, 'formation-slot-slot-research')
    await page.getByRole('menuitem', { name: 'Open terminal' }).click()
    await expect(page.getByTestId('formation-terminal-ghost')).toContainText('session is not live')
  })

  test('DECISIONS-LOCKED D7 + connections.feature: pointer ports wire, no-op, reconnect, reroute, reset, and remove', async ({ page }) => {
    const harness = await installPrototypeHarness(page)
    await mountFormations(page)

    await dragBetween(page, 'formation-output-formation-research-out', 'formation-input-formation-ship-in')
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'formation-research:out' && connection.to === 'formation-ship:in'
    ))).toBe(true)

    await dragBetween(page, 'formation-input-formation-ship-in', 'formation-input-gate-review-in')
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'formation-research:out' && connection.to === 'gate-review:in'
    ))).toBe(true)

    const beforeEmptyInput = harness.boardPatches.length
    const emptyInput = page.getByTestId('formation-input-formation-frame-in')
    const emptyBox = await emptyInput.boundingBox()
    expect(emptyBox).not.toBeNull()
    await page.mouse.move(emptyBox!.x + 6, emptyBox!.y + 6)
    await page.mouse.down()
    await page.mouse.move(emptyBox!.x + 80, emptyBox!.y + 20)
    await page.mouse.up()
    expect(harness.boardPatches.length).toBe(beforeEmptyInput)

    await dragHandle(page, 'formation-wire-edge-frame-research', 0, 90)
    await expect.poll(() => harness.currentLayout().edges.find(edge => edge.id === 'edge-frame-research')?.lane).toBe('manual')

    await openContextMenu(page, 'formation-wire-edge-frame-research')
    await page.getByRole('menuitem', { name: 'Reset routing' }).click()
    await expect.poll(() => harness.currentLayout().edges.find(edge => edge.id === 'edge-frame-research')?.lane).toBe('auto')

    await openContextMenu(page, 'formation-wire-edge-frame-research')
    await page.getByRole('menuitem', { name: 'Remove connection' }).click()
    await expect.poll(() => harness.currentBoard().connections.some(connection => connection.id === 'edge-frame-research')).toBe(false)
  })

  test('gates-and-judges.feature: pass/fail sockets, judge picker, auto-loop, chain, detach, and empty-canvas judge spawn are canvas gestures', async ({ page }) => {
    const harness = await installPrototypeHarness(page)
    await mountFormations(page)

    await dragBetween(page, 'gate-output-gate-review-pass', 'formation-input-formation-ship-in')
    await dragBetween(page, 'gate-output-gate-review-fail', 'formation-input-formation-frame-in')
    await expect.poll(() => harness.currentBoard().connections.some(connection => connection.from === 'gate-review:pass')).toBe(true)
    await expect.poll(() => harness.currentBoard().connections.some(connection => connection.from === 'gate-review:fail')).toBe(true)

    await clickAtCenter(page, 'gate-judge-socket-gate-review')
    await expect(page.getByRole('menu', { name: 'Judge socket actions' })).toBeVisible()
    await page.getByRole('menuitem', { name: 'New judge formation' }).click()
    await expect.poll(() => harness.currentBoard().formations.some(formation => formation.title === 'Judge formation')).toBe(true)
    await expect.poll(() => harness.currentBoard().connections.some(connection => connection.from === 'gate-review:judge')).toBe(true)
    await expect.poll(() => harness.currentBoard().connections.some(connection => connection.to === 'gate-review:judge')).toBe(true)

    await dragBetween(page, 'formation-output-formation-research-out', 'gate-judge-socket-gate-review')
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'formation-research:out' && connection.to === 'gate-review:judge'
    ))).toBe(true)

    await clickAtCenter(page, 'gate-judge-socket-gate-review')
    await page.getByRole('menuitem', { name: 'Use chain: Frame, Research, Ship' }).click()
    const lastJudgePatch = [...harness.boardPatches].reverse().find(hasGateJudgeChain)
    expect(lastJudgePatch?.setGateJudge.chain).toEqual(['formation-frame', 'formation-research', 'formation-ship'])

    await dragBetween(page, 'formation-output-formation-critique-out', 'gate-judge-socket-gate-review')
    const movedReturnPatch = [...harness.boardPatches].reverse().find(hasGateJudgeChain)
    expect(movedReturnPatch?.setGateJudge.chain).toEqual(['formation-frame', 'formation-research', 'formation-ship', 'formation-critique'])
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'gate-review:judge' && connection.to === 'formation-frame:in'
    ))).toBe(true)
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'formation-critique:out' && connection.to === 'gate-review:judge'
    ))).toBe(true)

    await clickAtCenter(page, 'gate-judge-socket-gate-review')
    await page.getByRole('menuitem', { name: 'Detach judge' }).click()
    await expect.poll(() => harness.currentBoard().connections.some(connection => (
      connection.from === 'gate-review:judge' || connection.to === 'gate-review:judge'
    ))).toBe(false)

    const gateBeforeSpawn = harness.currentLayout().nodes.find(node => node.id === 'gate-review')
    expect(gateBeforeSpawn, 'gate layout should exist before spawning a judge').not.toBeUndefined()
    const formationCountBeforeSpawn = harness.currentBoard().formations.length
    await dragToCanvasPoint(page, 'gate-judge-socket-gate-review', { x: 40, y: 80 })
    await expect.poll(() => harness.currentBoard().formations.length).toBe(formationCountBeforeSpawn + 1)
    const spawnedId = harness.currentBoard().formations[harness.currentBoard().formations.length - 1]?.id
    await expect.poll(() => harness.currentLayout().nodes.find(node => node.id === spawnedId)?.x).toBeLessThan((gateBeforeSpawn?.x || 0) - 500)
    await expect.poll(() => harness.currentLayout().nodes.find(node => node.id === spawnedId)?.y).toBeLessThan((gateBeforeSpawn?.y || 0) - 300)
  })

  test('frontend-prototype report: board-change truth reloads external edits and blocked runs survive tab reconnect', async ({ page }) => {
    const harness = await installPrototypeHarness(page, { enableBoardChange: true })
    await mountFormations(page)

    await expect(page.getByText('rev 3')).toBeVisible()
    await expect(page.getByText('rev 4')).toBeVisible()
    await expect(page.getByTestId('formation-wire-edge-external-ship')).toBeVisible()

    await page.getByLabel('Start Showcase mission').dispatchEvent('click')
    await expect(page.getByTestId('formation-run-status')).toContainText('run-prototype-red')
    await page.reload()
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formation-run-status')).toContainText('blocked')
    expect(harness.tmuxMutations).toEqual([])
  })
})

async function json(route: Route, data: object, headers?: Record<string, string>) {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    headers,
    body: JSON.stringify({ success: true, timestamp: new Date().toISOString(), data }),
  })
}

function withNextRev(board: Board): Board {
  const rev = board.rev + 1
  return { ...board, rev, etag: `"board-prototype-red-rev-${rev}"` }
}

function applyBoardPatch(board: Board, body: BoardPatch): Board {
  if (body.wireConnection) {
    const from = body.wireConnection.from
    const to = body.wireConnection.to
    const nextConnections = [
      ...board.connections.filter(connection => connection.to !== to && connection.from !== from),
      { id: `edge-${endpointID(from)}-${endpointID(to)}`, from, to },
    ]
    return withNextRev({ ...board, connections: nextConnections })
  }

  if (body.unwireConnection) {
    const from = body.unwireConnection.from
    const to = body.unwireConnection.to
    return withNextRev({
      ...board,
      connections: board.connections.filter(connection => connection.from !== from || connection.to !== to),
    })
  }

  if (body.rewireConnection) {
    const from = body.rewireConnection.from
    const previousTo = body.rewireConnection.previousTo
    const to = body.rewireConnection.to
    if (!board.connections.some(connection => connection.from === from && connection.to === previousTo)) return withNextRev(board)
    if (board.connections.some(connection => connection.to === to && !(connection.from === from && connection.to === previousTo))) return withNextRev(board)
    return withNextRev({
      ...board,
      connections: [
        ...board.connections.filter(connection => connection.from !== from || connection.to !== previousTo),
        { id: `edge-${endpointID(from)}-${endpointID(to)}`, from, to },
      ],
    })
  }

  if (body.setGateJudge) {
    const gateId = body.setGateJudge.gateId
    const chain = body.setGateJudge.chain || []
    const judgeConnections: Connection[] = []
    const addJudgeConnection = (from: string, to: string) => {
      if (judgeConnections.some(connection => connection.from === from && connection.to === to)) return
      if (board.connections.some(connection => (
        connection.from !== `${gateId}:judge` &&
        connection.to !== `${gateId}:judge` &&
        connection.from === from &&
        connection.to === to
      ))) return
      judgeConnections.push({ id: `edge-${endpointID(from)}-${endpointID(to)}`, from, to })
    }
    if (chain[0]) addJudgeConnection(`${gateId}:judge`, `${chain[0]}:in`)
    for (let i = 0; i < chain.length - 1; i += 1) {
      addJudgeConnection(`${chain[i]}:out`, `${chain[i + 1]}:in`)
    }
    if (chain.length > 0) {
      addJudgeConnection(`${chain[chain.length - 1]}:out`, `${gateId}:judge`)
    }
    return withNextRev({
      ...board,
      gates: board.gates.map(gate => gate.id === gateId
        ? { ...gate, kinds: Array.from(new Set([...gate.kinds, 'formation'])) }
        : gate),
      connections: [
        ...board.connections.filter(connection => connection.from !== `${gateId}:judge` && connection.to !== `${gateId}:judge`),
        ...judgeConnections,
      ],
    })
  }

  if (body.detachGateJudge) {
    const gateId = body.detachGateJudge.gateId
    return withNextRev({
      ...board,
      gates: board.gates.map(gate => gate.id === gateId
        ? { ...gate, kinds: gate.kinds.filter(kind => kind !== 'formation') }
        : gate),
      connections: board.connections.filter(connection => connection.from !== `${gateId}:judge` && connection.to !== `${gateId}:judge`),
    })
  }

  if (body.createFormation) {
    const generatedId = nextGeneratedFormationId(board, body.createFormation.title || 'Judge formation')
    const id = body.createFormation.id || generatedId
    return withNextRev({
      ...board,
      formations: [
        ...board.formations,
        {
          id,
          type: body.createFormation.type || 'solo',
          title: body.createFormation.title || 'Judge formation',
          inputs: [{ id: 'in', label: 'input' }],
          outputs: [{ id: 'out', label: 'output' }],
          slots: [{ id: 'slot-judge-spawned', label: 'Judge', controller: false }],
        },
      ],
    })
  }

  if (body.assignSlot) {
    const formationId = body.assignSlot.formationId
    const slotId = body.assignSlot.slotId
    const agentId = body.assignSlot.agentId || ''
    const harness = body.assignSlot.harness || ''
    return withNextRev({
      ...board,
      formations: board.formations.map(formation => formation.id === formationId
        ? {
          ...formation,
          slots: formation.slots.map(slot => slot.id === slotId
            ? {
              ...slot,
              agentId: agentId || undefined,
              harness: harness || undefined,
            }
            : slot),
        }
        : formation),
    })
  }

  if (body.setBrief) {
    const formationId = body.setBrief.formationId
    return withNextRev({
      ...board,
      formations: board.formations.map(formation => formation.id === formationId
        ? {
          ...formation,
          brief: {
            goal: body.setBrief?.goal || '',
            beadId: body.setBrief?.beadId || '',
            files: body.setBrief?.files || [],
            links: body.setBrief?.links || [],
          },
        }
        : formation),
    })
  }

  if (body.clearBrief) {
    const formationId = body.clearBrief.formationId
    return withNextRev({
      ...board,
      formations: board.formations.map(formation => {
        if (formation.id !== formationId) return formation
        const next = { ...formation }
        delete next.brief
        return next
      }),
    })
  }

  return withNextRev(board)
}

function applyLayoutPatch(layout: Layout, body: LayoutPatch): Layout {
  const rev = Number(layout.etag.match(/rev-(\d+)/)?.[1] || '3') + 1
  if (Array.isArray(body.edges)) {
    const patches = body.edges
    return {
      ...layout,
      etag: `"layout-prototype-red-rev-${rev}"`,
      edges: [
        ...layout.edges.filter(edge => !patches.some(patch => patch.id === edge.id)),
        ...patches,
      ],
    }
  }
  if (Array.isArray(body.nodes)) {
    const patches = body.nodes
    return {
      ...layout,
      etag: `"layout-prototype-red-rev-${rev}"`,
      nodes: [
        ...layout.nodes.filter(node => !patches.some(patch => patch.id === node.id)),
        ...patches,
      ],
    }
  }
  return { ...layout, etag: `"layout-prototype-red-rev-${rev}"` }
}

function endpointID(endpoint: string) {
  return endpoint.replace(/[^a-zA-Z0-9]+/g, '-').replace(/^-|-$/g, '')
}

function hasGateJudgeChain(patch: BoardPatch): patch is BoardPatch & { setGateJudge: GateJudgePatch & { chain: string[] } } {
  return Array.isArray(patch.setGateJudge?.chain)
}

function nextGeneratedFormationId(board: Board, title: string): string {
  const stem = title === 'Judge formation' ? 'formation-judge-spawned' : `formation-${endpointID(title.toLowerCase()) || 'created'}`
  if (!board.formations.some(formation => formation.id === stem)) return stem
  for (let index = 2; ; index += 1) {
    const candidate = `${stem}-${index}`
    if (!board.formations.some(formation => formation.id === candidate)) return candidate
  }
}
