import { allowBrowserConsoleMessage, test, expect } from '../fixtures'
import { mockApiRoutes, mockFormationsApiRoutes, mockFormationsBoard, mockFormationsLayout, mockFormationsAgents, mockFormationsRunEvents } from '../mock-api'
import type { Locator, Page, Route } from '@playwright/test'
import { activeRunStorageKey } from '../../src/components/formationsRunState'
import { createArchonPoemRoundTripFixture } from './archon-poem-fixture'

type CockpitRunStatus = {
  runId: string
  status: string
  final: boolean
  boardSlug: string
  missionId: string
  eventCount: number
  resumeAllowed?: boolean
  [key: string]: unknown
}

type CockpitRunEvent = {
  seq: number
  type: string
  runId: string
  nodeId?: string
  gateId?: string
  data?: Record<string, unknown>
  [key: string]: unknown
}

function apiBody(data: object) {
  return JSON.stringify({
    success: true,
    timestamp: new Date().toISOString(),
    data,
  })
}

async function requiredBox(locator: Locator, label: string) {
  const box = await locator.boundingBox()
  expect(box, `${label} should have a rendered box`).not.toBeNull()
  return {
    ...box!,
    right: box!.x + box!.width,
    bottom: box!.y + box!.height,
    centerX: box!.x + box!.width / 2,
    centerY: box!.y + box!.height / 2,
  }
}

function requiredString(value: unknown, label: string) {
  if (typeof value !== 'string' || value === '') throw new Error(`${label} missing from Archon fixture`)
  return value
}

function boxesOverlap(a: { left: number; right: number; top: number; bottom: number }, b: { left: number; right: number; top: number; bottom: number }) {
  return Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left)) *
    Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top)) > 0
}

function requiredArray(value: unknown, label: string): Record<string, unknown>[] {
  if (!Array.isArray(value)) throw new Error(`${label} missing from Archon fixture`)
  return value as Record<string, unknown>[]
}

async function fulfillApi(route: Route, data: object, headers?: Record<string, string>) {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    headers,
    body: apiBody(data),
  })
}

async function installArchonRunLifecycleHarness(page: Page, fixture: ReturnType<typeof createArchonPoemRoundTripFixture>) {
  const board = fixture.board as typeof mockFormationsBoard
  const layout = fixture.layout as typeof mockFormationsLayout
  const agents = fixture.agents as typeof mockFormationsAgents
  const gateId = requiredString(fixture.gate.id, 'gate id')
  let status = fixture.startedStatus as CockpitRunStatus
  let events = fixture.startedEvents as CockpitRunEvent[]

  await mockApiRoutes(page)
  await page.route('**/api/agents**', async route => {
    await fulfillApi(route, { agents, count: agents.length })
  })
  await page.route('**/api/formations/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname

    if (request.method() === 'GET' && path === '/api/formations/boards') {
      await fulfillApi(route, {
        boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }],
      })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}`) {
      await fulfillApi(route, { board }, { ETag: board.etag })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/layout`) {
      await fulfillApi(route, { layout }, { ETag: layout.etag })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/changes`) {
      await fulfillApi(route, { signal: { board: board.slug, changed: false, rev: board.rev, etag: board.etag } })
      return
    }
    if (request.method() === 'POST' && path === '/api/formations/runs') {
      status = fixture.startedStatus as CockpitRunStatus
      events = fixture.startedEvents as CockpitRunEvent[]
      await fulfillApi(route, { runId: status.runId, status })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/runs/${status.runId}`) {
      await fulfillApi(route, { status })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/runs/${status.runId}/events`) {
      await fulfillApi(route, { events })
      return
    }
    if (request.method() === 'POST' && path === `/api/formations/runs/${status.runId}/gates/${gateId}/verdict`) {
      status = fixture.approvedStatus as CockpitRunStatus
      events = fixture.approvedEvents as CockpitRunEvent[]
      await fulfillApi(route, { status })
      return
    }
    if (request.method() === 'POST' && path === `/api/formations/runs/${status.runId}/resume`) {
      status = fixture.finalStatus as CockpitRunStatus
      events = fixture.finalEvents as CockpitRunEvent[]
      await fulfillApi(route, { status })
      return
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ success: false, error: { code: 'MOCK_NOT_FOUND', message: `${request.method()} ${path}` } }),
    })
  })

  return { runId: status.runId, gateId, boardSlug: board.slug }
}

async function installReloadRecoveryHarness(page: Page) {
  let board = {
    ...mockFormationsBoard,
    slug: 'reload-poems',
    title: 'Reload poems',
  }
  let layout = {
    ...mockFormationsLayout,
    boardId: board.id,
  }
  const externalFormation = {
    id: 'formation-external-polish',
    type: 'solo' as const,
    title: 'External polish',
    inputs: [{ id: 'in', label: 'input' }],
    outputs: [{ id: 'out', label: 'output' }],
    slots: [{ id: 'slot-external', label: 'Polisher', controller: false, agentId: 'claude', harness: 'claude-code' }],
  }
  const changedBoard = {
    ...board,
    rev: board.rev + 1,
    etag: '"reload-poems-rev-4"',
    formations: [...board.formations, externalFormation],
    connections: [
      ...board.connections,
      { id: 'edge-external-polish', from: 'gate-review:pass', to: 'formation-external-polish:in' },
    ],
  }
  const changedLayout = {
    ...layout,
    boardRev: changedBoard.rev,
    etag: '"reload-poems-layout-rev-4"',
    nodes: [...layout.nodes, { id: externalFormation.id, x: 1060, y: 140 }],
  }
  const runId = 'run-reload-proof'
  const runStatus: CockpitRunStatus = {
    runId,
    status: 'blocked',
    final: false,
    boardSlug: board.slug,
    missionId: board.missions[0].id,
    eventCount: 2,
    resumeAllowed: true,
  }
  const runEvents: CockpitRunEvent[] = [
    { seq: 1, type: 'human_input_requested', runId, gateId: 'gate-review', data: { prompt: 'Approve the recovered draft' } },
    { seq: 2, type: 'run_blocked', runId, gateId: 'gate-review', data: { reason: 'waiting for human', resumeAllowed: true } },
  ]
  let changesServed = 0

  await mockApiRoutes(page)
  await page.route('**/api/agents**', async route => {
    await fulfillApi(route, { agents: mockFormationsAgents, count: mockFormationsAgents.length })
  })
  await page.route('**/api/formations/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (request.method() === 'GET' && path === '/api/formations/boards') {
      await fulfillApi(route, {
        boards: [{ id: board.id, slug: board.slug, title: board.title, rev: board.rev, etag: board.etag }],
      })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}`) {
      await fulfillApi(route, { board }, { ETag: board.etag })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/layout`) {
      await fulfillApi(route, { layout }, { ETag: layout.etag })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/changes`) {
      changesServed += 1
      if (changesServed === 1) {
        board = changedBoard
        layout = changedLayout
        await fulfillApi(route, { signal: { board: board.slug, changed: true, rev: board.rev, etag: board.etag } })
        return
      }
      await fulfillApi(route, { signal: { board: board.slug, changed: false, rev: board.rev, etag: board.etag } })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/runs/${runId}`) {
      await fulfillApi(route, { status: runStatus })
      return
    }
    if (request.method() === 'GET' && path === `/api/formations/runs/${runId}/events`) {
      await fulfillApi(route, { events: runEvents })
      return
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ success: false, error: { code: 'MOCK_NOT_FOUND', message: `${request.method()} ${path}` } }),
    })
  })

  return { runId, boardSlug: board.slug }
}

/**
 * Reference-grounded checks for the rebuilt Formations cockpit (FormationsCockpit),
 * asserting the D7 reference structure that the old form-style view lacked:
 * a left Agent Roster, circular slot spheres (not dropdowns), compact typed cards,
 * a spatial world with wires, and zoom controls. See Perttus_vision_for_agent_orchestration/03-formations.{html,js}.
 */
test.describe('Formations cockpit — D7 reference parity', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockFormationsApiRoutes(page)
    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toBeVisible()
  })

  test('renders the agent roster as a drag-source (was entirely absent before)', async ({ page }) => {
    const roster = page.getByTestId('agent-roster')
    await expect(roster).toBeVisible()
    for (const agent of mockFormationsAgents) {
      await expect(page.getByTestId(`roster-agent-${agent.id}`)).toBeVisible()
    }
    // roster cards are grab-draggable (cursor:grab), the reference's staffing gesture
    await expect(page.getByTestId(`roster-agent-${mockFormationsAgents[0].id}`)).toHaveCSS('cursor', 'grab')
  })

  test('staffs slots with circular spheres, not dropdowns', async ({ page }) => {
    const formation = mockFormationsBoard.formations[0]
    const card = page.getByTestId(`formation-node-${formation.id}`)
    await expect(card).toBeVisible()
    // compact card head with title + run button (not a tall form stack)
    await expect(card.locator('.fhead .tt')).toHaveText(formation.title)
    await expect(card.locator('.frun')).toBeVisible()
    // each slot is a sphere ring; the staffed ones show agent initials, no <select>
    const rings = card.locator('.slot .slot-ring')
    await expect(rings).toHaveCount(formation.slots.length)
    await expect(card.locator('select')).toHaveCount(0)
    await expect(card.locator('.slot.filled')).toHaveCount(formation.slots.filter(s => s.agentId).length)
  })

  test('renders mission + gate cards and a wire on a spatial world', async ({ page }) => {
    await expect(page.getByTestId(`mission-node-${mockFormationsBoard.missions[0].id}`).locator('.mtitle')).toHaveText(mockFormationsBoard.missions[0].title)
    await expect(page.getByTestId(`gate-node-${mockFormationsBoard.gates[0].id}`).locator('.gico')).toBeVisible()
    await expect(page.locator('.fmx .world .wires path.wire')).toHaveCount(mockFormationsBoard.connections.length)
    await expect(page.locator('.fmx .zoomctl')).toBeVisible()
  })

  test('keeps legacy inline verification legible without offering authoring', async ({ page }) => {
    const formation = mockFormationsBoard.formations[0]
    const band = page.getByRole('button', { name: `Inspect legacy verification for ${formation.title}` })
    await expect(band).toBeVisible()
    await band.click()

    const dialog = page.getByRole('dialog', { name: `Legacy verification · ${formation.title}` })
    await expect(dialog).toHaveAttribute('aria-modal', 'true')
    await expect(dialog).toHaveAccessibleDescription(/Inline verification is retired/)
    await expect(dialog.getByRole('button', { name: 'Close legacy verification' })).toBeFocused()
    await expect(dialog).toContainText(formation.verification.criterion)
    await expect(dialog).toContainText('Create and wire an explicit Gate')
    await expect(dialog.getByRole('button', { name: 'Save verification' })).toHaveCount(0)
    await expect(dialog.getByLabel('Replacement Gate')).toHaveValue('')
    await expect(dialog.getByRole('button', { name: 'Remove legacy verification' })).toBeDisabled()
    await dialog.getByLabel('Replacement Gate').selectOption(mockFormationsBoard.gates[0].id)
    await expect(dialog.getByRole('button', { name: 'Remove legacy verification' })).toBeEnabled()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await expect(band).toBeFocused()
  })

  test('announces a redaction-safe dialog-local removal failure', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 409')
    await page.route('**/api/formations/**', async route => {
      const request = route.request()
      const body = request.method() === 'PATCH' ? request.postDataJSON() as Record<string, unknown> : null
      if (body?.removeVerification) {
        await route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({ success: false, error: { code: 'WRITE_REJECTED', message: 'sensitive backend migration detail' } }),
        })
        return
      }
      await route.fallback()
    })

    const formation = mockFormationsBoard.formations[0]
    await page.getByTestId(`verify-band-${formation.id}`).click()
    const dialog = page.getByRole('dialog', { name: `Legacy verification · ${formation.title}` })
    await dialog.getByLabel('Replacement Gate').selectOption(mockFormationsBoard.gates[0].id)
    await dialog.getByRole('button', { name: 'Remove legacy verification' }).click()

    const localError = dialog.getByRole('alert')
    await expect(localError).toContainText('Could not remove legacy verification')
    await expect(localError).not.toContainText('sensitive backend migration detail')
    await expect(dialog).toBeVisible()
  })

  test('locks duplicate submission until successful removal restores trigger focus', async ({ page }) => {
    let releaseRemoval: (() => void) | undefined
    const removalGate = new Promise<void>(resolve => { releaseRemoval = resolve })
    let removalRequests = 0
    await page.route('**/api/formations/**', async route => {
      const request = route.request()
      const body = request.method() === 'PATCH' ? request.postDataJSON() as Record<string, unknown> : null
      if (body?.removeVerification) {
        removalRequests += 1
        await removalGate
        const board = { ...mockFormationsBoard, rev: mockFormationsBoard.rev + 1, etag: 'migration-success-etag' }
        await fulfillApi(route, { board, layout: null }, { ETag: board.etag })
        return
      }
      await route.fallback()
    })

    const formation = mockFormationsBoard.formations[0]
    const band = page.getByTestId(`verify-band-${formation.id}`)
    await band.click()
    const dialog = page.getByRole('dialog', { name: `Legacy verification · ${formation.title}` })
    const replacement = dialog.getByLabel('Replacement Gate')
    const remove = dialog.getByRole('button', { name: 'Remove legacy verification' })
    await replacement.selectOption(mockFormationsBoard.gates[0].id)
    await remove.click()

    await expect(dialog.getByRole('status')).toContainText('Removing legacy verification')
    await expect(replacement).toBeDisabled()
    await expect(remove).toBeDisabled()
    expect(removalRequests).toBe(1)

    releaseRemoval?.()
    await expect(dialog).toBeHidden()
    await expect(band).toBeFocused()
    expect(removalRequests).toBe(1)
  })

  test('closes on board change and restores focus when the migration trigger remains', async ({ page }) => {
    const secondBoard = {
      ...mockFormationsBoard,
      id: 'board-second',
      slug: 'second-board',
      title: 'Second board',
      rev: mockFormationsBoard.rev + 1,
      etag: 'second-board-etag',
    }
    const secondLayout = { ...mockFormationsLayout, board: secondBoard.slug, boardRev: secondBoard.rev, etag: 'second-layout-etag' }
    await page.route('**/api/formations/**', async route => {
      const request = route.request()
      const path = new URL(request.url()).pathname
      if (request.method() === 'GET' && path === '/api/formations/boards') {
        await fulfillApi(route, { boards: [
          { id: mockFormationsBoard.id, slug: mockFormationsBoard.slug, title: mockFormationsBoard.title, rev: mockFormationsBoard.rev, etag: mockFormationsBoard.etag },
          { id: secondBoard.id, slug: secondBoard.slug, title: secondBoard.title, rev: secondBoard.rev, etag: secondBoard.etag },
        ] })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${secondBoard.slug}`) {
        await fulfillApi(route, { board: secondBoard }, { ETag: secondBoard.etag })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${secondBoard.slug}/layout`) {
        await fulfillApi(route, { layout: secondLayout }, { ETag: secondLayout.etag })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${secondBoard.slug}/changes`) {
        await fulfillApi(route, { signal: { board: secondBoard.slug, changed: false, rev: secondBoard.rev, etag: secondBoard.etag } })
        return
      }
      await route.fallback()
    })

    await page.reload()
    await page.getByRole('button', { name: 'Formations' }).click()
    const band = page.getByTestId(`verify-band-${mockFormationsBoard.formations[0].id}`)
    await band.click()
    const dialog = page.getByRole('dialog', { name: `Legacy verification · ${mockFormationsBoard.formations[0].title}` })
    await dialog.getByLabel('Replacement Gate').focus()
    await page.getByTestId('board-picker').selectOption(secondBoard.slug)

    await expect(dialog).toBeHidden()
    await expect(page.getByTestId(`verify-band-${mockFormationsBoard.formations[0].id}`)).toBeFocused()
  })

  test('matches the 03-formations.html first-viewport cockpit geometry', async ({ page }) => {
    const cockpit = page.locator('.fmx[data-cockpit="d7"]')
    const topbar = page.locator('.fmx .topbar')
    const roster = page.getByTestId('agent-roster')
    const viewport = page.locator('.fmx .viewport')
    const world = page.locator('.fmx .world')
    const mission = page.getByTestId(`mission-node-${mockFormationsBoard.missions[0].id}`)
    const formation = page.getByTestId(`formation-node-${mockFormationsBoard.formations[0].id}`)
    const gate = page.getByTestId(`gate-node-${mockFormationsBoard.gates[0].id}`)

    await expect(cockpit).toHaveAttribute('data-cockpit', 'd7')
    await expect(cockpit).toHaveCSS('background-color', 'rgb(15, 15, 15)')
    await expect(world).toHaveCSS('background-size', '112px 112px, 112px 112px, 28px 28px')

    const topbarBox = await requiredBox(topbar, 'D7 topbar')
    const rosterBox = await requiredBox(roster, 'D7 roster')
    const viewportBox = await requiredBox(viewport, 'D7 viewport')
    const missionBox = await requiredBox(mission, 'D7 mission card')
    const formationBox = await requiredBox(formation, 'D7 formation card')
    const gateBox = await requiredBox(gate, 'D7 gate card')

    // Cockpit-density toolbar (no wordmark row): slimmer than the prototype's.
    expect(topbarBox.height).toBeGreaterThanOrEqual(38)
    expect(topbarBox.height).toBeLessThanOrEqual(64)
    expect(Math.round(rosterBox.width)).toBe(236)
    expect(Math.abs(rosterBox.y - topbarBox.bottom)).toBeLessThanOrEqual(2)
    expect(Math.abs(viewportBox.x - rosterBox.right)).toBeLessThanOrEqual(2)
    expect(Math.abs(viewportBox.y - topbarBox.bottom)).toBeLessThanOrEqual(2)

    await expect(mission).toHaveCSS('width', '236px')
    expect(formationBox.width).toBeGreaterThanOrEqual(300)
    expect(formationBox.width).toBeLessThanOrEqual(430)
    expect(gateBox.width).toBeGreaterThanOrEqual(150)
    expect(gateBox.width).toBeLessThanOrEqual(300)
    expect(missionBox.x).toBeLessThan(formationBox.x)
    expect(formationBox.x).toBeLessThan(gateBox.x)
    expect(formationBox.x - missionBox.right).toBeLessThanOrEqual(150)
    expect(gateBox.x - formationBox.right).toBeLessThanOrEqual(150)
    expect(Math.abs(missionBox.y - formationBox.y)).toBeLessThanOrEqual(70)
    expect(Math.abs(formationBox.y - gateBox.y)).toBeLessThanOrEqual(70)
    expect(Math.abs(missionBox.centerY - formationBox.centerY)).toBeLessThanOrEqual(220)
    expect(Math.abs(formationBox.centerY - gateBox.centerY)).toBeLessThanOrEqual(220)
    expect(missionBox.x).toBeGreaterThanOrEqual(viewportBox.x + 16)
    expect(gateBox.right).toBeLessThanOrEqual(viewportBox.right - 16)

    await expect(formation.locator('.slot-ring').first()).toHaveCSS('border-radius', '50%')
    await expect(gate.locator('.glabel.pass')).toBeVisible()
    await expect(gate.locator('.glabel.fail')).toBeVisible()
    await expect(gate.locator('.pjudge')).toBeVisible()
    await expect(page.locator('.fmx .wires path.wire')).toHaveCount(mockFormationsBoard.connections.length)
    await page.screenshot({ path: '/tmp/chrote-gap-loop/reports/CODEX_UI_REFERENCE_FIX_COCKPIT.png' })
  })
})

test.describe('Formations cockpit — Tool projection', () => {
  test('keeps non-executing Tools legible through inspection, routing, FIT, drag, and text sizing', async ({ page }) => {
    const sourceTool = {
      id: 'tool_source',
      title: 'Source JSON',
      profileId: 'json.normalize',
      profileVersion: '1',
      params: { mode: 'strict' },
      inputs: [{
        id: 'in',
        name: 'input',
        label: 'Source input',
        direction: 'input' as const,
        kind: 'work' as const,
        acceptedMediaTypes: ['application/json'],
        required: true,
        role: 'data' as const,
      }],
      outputs: [{
        id: 'out',
        name: 'output',
        label: 'Source output',
        direction: 'output' as const,
        kind: 'work' as const,
        acceptedMediaTypes: ['application/json'],
      }],
    }
    const normalizeTool = {
      ...sourceTool,
      id: 'tool_normalize',
      title: 'Normalize report',
      inputs: sourceTool.inputs.map(port => ({ ...port, label: 'Report' })),
      outputs: sourceTool.outputs.map(port => ({ ...port, label: 'Normalized report' })),
    }
    const board = {
      ...mockFormationsBoard,
      schema: 2,
      tools: [sourceTool, normalizeTool],
      connections: [
        ...mockFormationsBoard.connections.filter(connection => connection.id !== 'conn-review-gate'),
        { id: 'edge_tool_gate', from: 'tool_normalize:out', to: 'gate-review:in' },
      ],
    }
    let currentBoard = board
    let currentLayout = {
      ...mockFormationsLayout,
      nodes: [
        ...mockFormationsLayout.nodes,
        { id: 'tool_source', x: 1200, y: 90 },
        { id: 'tool_normalize', x: 1600, y: 150 },
      ],
    }
    const mutations: Array<{ method: string; path: string; body: Record<string, unknown> }> = []

    const fontPx = (locator: Locator) => locator.evaluate(element => Number.parseFloat(getComputedStyle(element).fontSize))
    const wireScreenEndpoints = (locator: Locator) => locator.evaluate((node: SVGPathElement) => {
      const length = node.getTotalLength()
      const matrix = node.getScreenCTM()
      if (!matrix) throw new Error('Tool wire has no screen transform')
      const start = node.getPointAtLength(0).matrixTransform(matrix)
      const end = node.getPointAtLength(length).matrixTransform(matrix)
      return { length, start: { x: start.x, y: start.y }, end: { x: end.x, y: end.y } }
    })
    const assertWire = async (wire: Locator, output: Locator, input: Locator) => {
      const endpoints = await wireScreenEndpoints(wire)
      const outputBox = await requiredBox(output, 'Tool output port')
      const inputBox = await requiredBox(input, 'Tool input port')
      expect(endpoints.length).toBeGreaterThan(20)
      expect(Math.abs(endpoints.start.x - outputBox.centerX)).toBeLessThanOrEqual(4)
      expect(Math.abs(endpoints.start.y - outputBox.centerY)).toBeLessThanOrEqual(4)
      expect(Math.abs(endpoints.end.x - inputBox.centerX)).toBeLessThanOrEqual(4)
      expect(Math.abs(endpoints.end.y - inputBox.centerY)).toBeLessThanOrEqual(4)
    }
    const openInspector = async () => {
      const dialog = page.getByRole('dialog', { name: 'Tool details: Normalize report' })
      if (!await dialog.isVisible()) {
        await page.getByRole('button', { name: 'Inspect Tool Normalize report' }).click()
      }
      await expect(dialog).toBeVisible()
      return dialog
    }

    await mockApiRoutes(page)
    await mockFormationsApiRoutes(page, { board, layout: currentLayout })
    await page.route('**/api/formations/**', async route => {
      const request = route.request()
      if (request.method() === 'GET') {
        await route.fallback()
        return
      }
      const path = new URL(request.url()).pathname
      const body = (request.postDataJSON() || {}) as Record<string, unknown>
      mutations.push({ method: request.method(), path, body })
      if (request.method() === 'PATCH' && path === `/api/formations/boards/${board.slug}`) {
        const connection = body.wireConnection as { from?: string; to?: string } | undefined
        if (connection?.from === 'tool_source:out' && connection.to === 'tool_normalize:in') {
          currentBoard = {
            ...currentBoard,
            rev: currentBoard.rev + 1,
            etag: '"board-tool-wire"',
            connections: [
              ...currentBoard.connections,
              { id: 'edge_tool_chain', from: connection.from, to: connection.to },
            ],
          }
          await fulfillApi(route, { board: currentBoard, layout: null }, { ETag: currentBoard.etag })
          return
        }
      }
      if (request.method() === 'PATCH' && path === `/api/formations/boards/${board.slug}/layout`) {
        const requestedNodes = Array.isArray(body.nodes) ? body.nodes as Array<{ id: string; x: number; y: number }> : []
        currentLayout = {
          ...currentLayout,
          etag: '"layout-tool-drag"',
          nodes: currentLayout.nodes.map(node => requestedNodes.find(requested => requested.id === node.id) || node),
        }
        await fulfillApi(route, { layout: currentLayout }, { ETag: currentLayout.etag })
        return
      }
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: { code: 'UNEXPECTED_MUTATION', message: `${request.method()} ${path}` } }),
      })
    })

    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toBeVisible()

    const sourceCard = page.getByTestId('tool-node-tool_source')
    const normalizeCard = page.getByTestId('tool-node-tool_normalize')
    await expect(sourceCard).toBeVisible()
    await expect(normalizeCard).toBeVisible()
    await expect(normalizeCard).toContainText('json.normalize@1')
    await expect(normalizeCard).toContainText('execution unavailable')
    const inspector = await openInspector()
    await expect(inspector).toContainText('tool_normalize')
    await expect(inspector).toContainText('mode')
    await expect(inspector).toContainText('strict')
    await expect(inspector).toContainText('Report')
    await expect(inspector).toContainText('Normalized report')

    const defaultFonts = {
      title: await fontPx(normalizeCard.locator('.tool-title')),
      profile: await fontPx(normalizeCard.locator('.tool-profile')),
      port: await fontPx(normalizeCard.locator('.tool-port-label').first()),
      detail: await fontPx(inspector.locator('.tool-detail-port').first()),
    }
    await inspector.getByRole('button', { name: 'Close Tool details' }).click()

    const world = page.getByTestId('formations-world')
    await page.getByRole('button', { name: 'FIT' }).click()
    await expect(world).toHaveClass(/smooth/)
    await expect(world).not.toHaveClass(/smooth/, { timeout: 1_000 })
    const viewportBox = await requiredBox(page.getByTestId('formations-canvas'), 'Formations viewport')
    for (const [label, card] of [['source Tool', sourceCard], ['normalize Tool', normalizeCard]] as const) {
      const cardBox = await requiredBox(card, label)
      expect(cardBox.x).toBeGreaterThanOrEqual(viewportBox.x)
      expect(cardBox.right).toBeLessThanOrEqual(viewportBox.right)
      expect(cardBox.y).toBeGreaterThanOrEqual(viewportBox.y)
      expect(cardBox.bottom).toBeLessThanOrEqual(viewportBox.bottom)
    }

    const normalizeCardBox = await requiredBox(normalizeCard.locator('.tool-title'), 'Tool judge-drop target')
    const judgeSocketBox = await requiredBox(page.getByTestId('gate-judge-socket-gate-review'), 'Gate judge socket')
    await page.mouse.move(judgeSocketBox.centerX, judgeSocketBox.centerY)
    await page.mouse.down()
    await page.mouse.move(judgeSocketBox.centerX + 6, judgeSocketBox.centerY + 6)
    await page.mouse.move(normalizeCardBox.centerX, normalizeCardBox.centerY, { steps: 10 })
    await page.mouse.up()
    expect(mutations).toHaveLength(0)

    const sourceOutput = page.locator('[data-port-out="tool_source:out"]')
    const normalizeInput = page.locator('[data-port-in="tool_normalize:in"]')
    const sourceOutputBox = await requiredBox(sourceOutput, 'Source Tool output')
    const normalizeInputBox = await requiredBox(normalizeInput, 'Normalize Tool input')
    await page.mouse.move(sourceOutputBox.centerX, sourceOutputBox.centerY)
    await page.mouse.down()
    await page.mouse.move(sourceOutputBox.centerX + 6, sourceOutputBox.centerY + 6)
    await page.mouse.move(normalizeInputBox.centerX, normalizeInputBox.centerY, { steps: 10 })
    await page.mouse.up()
    await expect.poll(() => mutations.length).toBe(1)
    expect(mutations[0]).toMatchObject({
      method: 'PATCH',
      path: `/api/formations/boards/${board.slug}`,
      body: { wireConnection: { from: 'tool_source:out', to: 'tool_normalize:in' } },
    })
    await expect(page.getByTestId('formation-wire-edge_tool_chain')).toBeAttached()
    await assertWire(page.getByTestId('formation-wire-edge_tool_chain'), sourceOutput, normalizeInput)
    await assertWire(
      page.getByTestId('formation-wire-edge_tool_gate'),
      page.locator('[data-port-out="tool_normalize:out"]'),
      page.locator('[data-port-in="gate-review:in"]'),
    )

    const worldTransform = await world.evaluate(element => (element as HTMLElement).style.transform)
    const dragHandle = await requiredBox(normalizeCard.locator('.tool-title'), 'Tool drag handle')
    await page.mouse.move(dragHandle.centerX, dragHandle.centerY)
    await page.mouse.down()
    await page.mouse.move(dragHandle.centerX + 6, dragHandle.centerY + 6)
    await page.mouse.move(dragHandle.centerX + 74, dragHandle.centerY + 38, { steps: 10 })
    await expect(world).toHaveClass(/nodedrag/)
    await expect(page.getByTestId('formations-canvas')).not.toHaveClass(/panning/)
    await page.mouse.up()
    await expect.poll(() => mutations.length).toBe(2)
    expect(mutations[1]).toMatchObject({
      method: 'PATCH',
      path: `/api/formations/boards/${board.slug}/layout`,
      body: { nodes: [{ id: 'tool_normalize', x: expect.any(Number), y: expect.any(Number) }] },
    })
    expect(await world.evaluate(element => (element as HTMLElement).style.transform)).toBe(worldTransform)

    await normalizeCard.click({ button: 'right' })
    await expect(page.locator('.fmx .ctxmenu')).toHaveCount(0)
    expect(mutations).toHaveLength(2)

    await page.getByRole('button', { name: 'Settings' }).click()
    await page.getByTestId('formations-textsize-large').click()
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toHaveAttribute('data-textsize', 'large')
    const largeInspector = await openInspector()
    const largeFonts = {
      title: await fontPx(normalizeCard.locator('.tool-title')),
      profile: await fontPx(normalizeCard.locator('.tool-profile')),
      port: await fontPx(normalizeCard.locator('.tool-port-label').first()),
      detail: await fontPx(largeInspector.locator('.tool-detail-port').first()),
    }
    for (const key of Object.keys(defaultFonts) as Array<keyof typeof defaultFonts>) {
      expect(largeFonts[key], `large ${key} text`).toBeGreaterThan(defaultFonts[key])
    }

    await page.getByRole('button', { name: 'Settings' }).click()
    await page.getByTestId('formations-textsize-xlarge').click()
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toHaveAttribute('data-textsize', 'xlarge')
    const xlargeInspector = await openInspector()
    const xlargeFonts = {
      title: await fontPx(normalizeCard.locator('.tool-title')),
      profile: await fontPx(normalizeCard.locator('.tool-profile')),
      port: await fontPx(normalizeCard.locator('.tool-port-label').first()),
      detail: await fontPx(xlargeInspector.locator('.tool-detail-port').first()),
    }
    for (const key of Object.keys(largeFonts) as Array<keyof typeof largeFonts>) {
      expect(xlargeFonts[key], `xlarge ${key} text`).toBeGreaterThan(largeFonts[key])
    }
    expect(mutations).toHaveLength(2)
  })
})

test.describe('Formations cockpit — layout safety', () => {
  test('preserves overlapping saved coordinates while keeping the mission start control reachable', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFormationsApiRoutes(page, {
      layout: {
        ...mockFormationsLayout,
        etag: '"layout-overlap-regression"',
        nodes: mockFormationsLayout.nodes.map(node => ({ ...node, x: 220, y: 160 })),
      },
    })
    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toBeVisible()

    const positions = await page.locator('.fmx .world .formation,.fmx .world .gatecard,.fmx .world .missioncard').evaluateAll(elements => elements.map(element => ({
      left: (element as HTMLElement).style.left,
      top: (element as HTMLElement).style.top,
    })))
    expect(positions).toHaveLength(mockFormationsBoard.missions.length + mockFormationsBoard.formations.length + mockFormationsBoard.gates.length)
    expect(new Set(positions.map(position => `${position.left}:${position.top}`))).toEqual(new Set(['220px:160px']))

    const runButton = page.getByTestId(`run-mission-${mockFormationsBoard.missions[0].id}`)
    const runBox = await requiredBox(runButton, 'overlapping mission Run control')
    const hitTarget = await page.evaluate(({ x, y }) => {
      const target = document.elementFromPoint(x, y)
      return target?.closest<HTMLElement>('[data-testid]')?.dataset.testid ?? null
    }, { x: runBox.centerX, y: runBox.centerY })
    expect(hitTarget).toBe(`run-mission-${mockFormationsBoard.missions[0].id}`)

    await runButton.click()
    await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('blocked')
  })
})

test.describe('Formations cockpit — responsive safety', () => {
  for (const viewport of [
    { label: 'desktop', width: 1280, height: 720, minCanvasWidth: 720, artifact: '/tmp/chrote-gap-loop/reports/CODEX_FINAL_CONFORMANCE_DESKTOP.png' },
    { label: 'mobile', width: 390, height: 844, minCanvasWidth: 180, artifact: '/tmp/chrote-gap-loop/reports/CODEX_FINAL_CONFORMANCE_MOBILE.png' },
  ]) {
    test(`keeps ${viewport.label} cockpit controls reachable without a blank canvas`, async ({ page }) => {
      await page.setViewportSize({ width: viewport.width, height: viewport.height })
      await mockApiRoutes(page)
      await mockFormationsApiRoutes(page)
      const layoutWrites: Record<string, unknown>[] = []
      await page.route('**/api/formations/**', async route => {
        if (route.request().method() === 'PATCH') {
          layoutWrites.push(route.request().postDataJSON() as Record<string, unknown>)
        }
        await route.fallback()
      })
      await page.goto('/')
      if (viewport.width <= 768) {
        await page.locator('.hamburger-btn').click()
        await page.getByRole('button', { name: 'Formations' }).click()
      } else {
        await page.getByRole('button', { name: 'Formations' }).click()
      }
      await expect(page.getByTestId('formations-view')).toBeVisible()

      const cockpit = page.locator('.fmx[data-cockpit="d7"]')
      const viewportBox = await requiredBox(page.locator('.fmx .viewport'), `${viewport.label} viewport`)
      const viewportRect = { left: viewportBox.x, right: viewportBox.right, top: viewportBox.y, bottom: viewportBox.bottom }
      const zoomBox = await requiredBox(page.locator('.fmx .zoomctl'), `${viewport.label} zoom controls`)
      const gateTokenBox = await requiredBox(page.getByTestId('gate-token'), `${viewport.label} gate token`)
      const newFormationBox = await requiredBox(page.getByTestId('new-formation'), `${viewport.label} new formation`)
      const boardPickerBox = await requiredBox(page.getByTestId('board-picker'), `${viewport.label} board picker`)
      const mission = page.getByTestId(`mission-node-${mockFormationsBoard.missions[0].id}`)
      const nodes = await page.locator('.fmx .world .formation,.fmx .world .gatecard,.fmx .world .missioncard').evaluateAll(elements => elements.map(element => {
        const rect = element.getBoundingClientRect()
        return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom }
      }))

      await expect(cockpit).toHaveAttribute('data-cockpit', 'd7')
      await expect(page.locator('.fmx .world')).toHaveCSS('background-size', '112px 112px, 112px 112px, 28px 28px')
      await expect(page.locator('.fmx .world .wires path.wire')).toHaveCount(mockFormationsBoard.connections.length)
      await expect(page.getByTestId('agent-roster')).toBeVisible()
      await expect(page.getByTestId(`roster-agent-${mockFormationsAgents[0].id}`)).toBeVisible()
      await expect(page.getByRole('button', { name: 'FIT' })).toBeVisible()

      expect(viewportBox.width, `${viewport.label} canvas width`).toBeGreaterThanOrEqual(viewport.minCanvasWidth)
      expect(viewportBox.height, `${viewport.label} canvas height`).toBeGreaterThanOrEqual(420)
      for (const [label, box] of [
        ['zoom controls', zoomBox],
        ['gate token', gateTokenBox],
        ['new formation', newFormationBox],
        ['board picker', boardPickerBox],
      ] as const) {
        expect(box.x, `${viewport.label} ${label} left`).toBeGreaterThanOrEqual(0)
        expect(box.right, `${viewport.label} ${label} right`).toBeLessThanOrEqual(viewport.width)
        expect(box.y, `${viewport.label} ${label} top`).toBeGreaterThanOrEqual(0)
        expect(box.bottom, `${viewport.label} ${label} bottom`).toBeLessThanOrEqual(viewport.height)
      }
      expect(boxesOverlap(zoomBox, gateTokenBox), `${viewport.label} zoom/gate overlap`).toBe(false)
      expect(boxesOverlap(zoomBox, newFormationBox), `${viewport.label} zoom/new overlap`).toBe(false)

      const visibleNodes = nodes.filter(box => boxesOverlap(box, viewportRect))
      expect(visibleNodes.length, `${viewport.label} should show at least one card on the canvas`).toBeGreaterThan(0)

      if (viewport.label === 'mobile') {
        // Supported narrow model: full-width horizontal agent rail above a
        // full-size pannable canvas. Controls stay at readable/actionable size;
        // the board is traversed by pan/zoom rather than whole-board shrinking.
        const rosterBox = await requiredBox(page.getByTestId('agent-roster'), 'mobile agent rail')
        expect(viewportBox.width).toBeGreaterThanOrEqual(360)
        expect(rosterBox.width).toBeGreaterThanOrEqual(360)
        expect(rosterBox.height).toBeGreaterThanOrEqual(72)
        expect(rosterBox.height).toBeLessThanOrEqual(110)

        await expect(page.locator('.fmx .zoomlevel')).toHaveText('100%')
        const missionBox = await requiredBox(mission, 'mobile mission card')
        const missionRunBox = await requiredBox(page.getByTestId(`run-mission-${mockFormationsBoard.missions[0].id}`), 'mobile mission action')
        expect(missionBox.width).toBeGreaterThanOrEqual(230)
        expect(missionRunBox.width).toBeGreaterThanOrEqual(34)
        expect(missionRunBox.height).toBeGreaterThanOrEqual(34)
        expect(boardPickerBox.width).toBeGreaterThanOrEqual(150)
        expect(boardPickerBox.height).toBeGreaterThanOrEqual(34)
        expect(newFormationBox.width).toBeGreaterThanOrEqual(110)
        expect(gateTokenBox.width).toBeGreaterThanOrEqual(60)
        await expect(page.getByTestId('new-formation')).toHaveCSS('font-size', '11px')
        await expect(page.getByTestId('gate-token')).toHaveCSS('font-size', '10px')

        const savedPositions = await page.locator('.fmx [data-node]').evaluateAll(elements => elements.map(element => ({
          id: (element as HTMLElement).dataset.node,
          left: (element as HTMLElement).style.left,
          top: (element as HTMLElement).style.top,
        })))
        expect(savedPositions).toEqual(expect.arrayContaining(mockFormationsLayout.nodes.map(node => ({
          id: node.id,
          left: `${node.x}px`,
          top: `${node.y}px`,
        }))))

        await page.getByTestId(`run-mission-${mockFormationsBoard.missions[0].id}`).click()
        await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('blocked')
        const runBannerBox = await requiredBox(page.getByTestId('run-banner'), 'mobile run banner')
        const runBadgeBox = await requiredBox(page.getByTestId('run-banner').locator('.badge'), 'mobile run badge')
        expect(runBannerBox.x).toBeGreaterThanOrEqual(viewportRect.left + 8)
        expect(runBannerBox.right).toBeLessThanOrEqual(viewportRect.right - 8)
        expect(runBadgeBox.x).toBeGreaterThanOrEqual(runBannerBox.x + 8)
        await page.getByTitle('Zoom in').click()
        await expect(page.locator('.fmx .zoomlevel')).toHaveText('120%')
        await page.getByRole('button', { name: 'FIT' }).click()
        await expect(page.locator('.fmx .zoomlevel')).toHaveText('100%')

        await page.mouse.move(viewportBox.centerX, viewportBox.bottom - 24)
        await page.mouse.down()
        await page.mouse.move(viewportBox.x + 40, viewportBox.bottom - 24, { steps: 8 })
        await page.mouse.up()
        const formation = page.getByTestId(`formation-node-${mockFormationsBoard.formations[0].id}`)
        const formationBox = await requiredBox(formation, 'mobile panned formation')
        expect(boxesOverlap({ left: formationBox.x, right: formationBox.right, top: formationBox.y, bottom: formationBox.bottom }, viewportRect)).toBe(true)
        await formation.getByTestId(`verify-band-${mockFormationsBoard.formations[0].id}`).click()
        await expect(page.getByRole('dialog', { name: `Legacy verification · ${mockFormationsBoard.formations[0].title}` })).toBeVisible()
        await page.getByRole('button', { name: 'Close legacy verification' }).click()
        const runBannerOverflow = await page.getByTestId('run-banner').evaluate(element => ({
          clientWidth: element.clientWidth,
          scrollLeft: element.scrollLeft,
          scrollWidth: element.scrollWidth,
        }))
        expect(runBannerOverflow.scrollLeft).toBe(0)
        expect(runBannerOverflow.scrollWidth).toBeLessThanOrEqual(runBannerOverflow.clientWidth)
        expect(layoutWrites).toEqual([])
      }

      await page.screenshot({ path: viewport.artifact })
    })
  }
})

test.describe('Formations cockpit — Archon round-trip projection', () => {
  test('renders an Archon-authored poem mission and projects its ledger-blocked gate state', async ({ page }) => {
    let fixture: ReturnType<typeof createArchonPoemRoundTripFixture> | null = null
    try {
      fixture = createArchonPoemRoundTripFixture()
      const missionId = requiredString(fixture.mission.id, 'mission id')
      const draftId = requiredString(fixture.draft.id, 'draft formation id')
      const polishId = requiredString(fixture.polish.id, 'polish formation id')
      const gateId = requiredString(fixture.gate.id, 'gate id')
      const draftSlotId = requiredString(requiredArray(fixture.draft.slots, 'draft slots')[0]?.id, 'draft slot id')
      const connectionCount = requiredArray(fixture.board.connections, 'board connections').length
      const layoutNodes = requiredArray(fixture.layout.nodes, 'layout nodes')

      expect(layoutNodes.some(node => node.id === draftId)).toBe(true)
      expect(layoutNodes.some(node => node.id === polishId)).toBe(true)
      expect(fixture.board).not.toHaveProperty('nodes')
      expect(fixture.runStatus).toMatchObject({
        runId: requiredString(fixture.runStatus.runId, 'run id'),
        status: 'blocked',
        final: false,
        boardSlug: 'poems',
        missionId,
        resumeAllowed: true,
        eventCount: fixture.runEvents.length,
      })

      await mockApiRoutes(page)
      await mockFormationsApiRoutes(page, {
        board: fixture.board as typeof mockFormationsBoard,
        layout: fixture.layout as typeof mockFormationsLayout,
        agents: fixture.agents as typeof mockFormationsAgents,
        runEvents: fixture.runEvents as typeof mockFormationsRunEvents,
        runStatus: fixture.runStatus as {
          runId: string
          status: string
          final: boolean
          boardSlug: string
          missionId: string
          eventCount: number
          resumeAllowed: boolean
        },
      })
      await page.goto('/')
      await page.getByRole('button', { name: 'Formations' }).click()
      await expect(page.getByTestId('formations-view')).toBeVisible()

      await expect(page.getByTestId('board-picker')).toHaveValue('poems')
      await expect(page.getByTestId(`mission-node-${missionId}`).locator('.mtitle')).toHaveText('Simple poem')
      await expect(page.getByTestId(`formation-node-${draftId}`).locator('.tt')).toHaveText('Draft poem')
      await expect(page.getByTestId(`formation-node-${polishId}`).locator('.tt')).toHaveText('Polish poem')
      await expect(page.getByTestId(`gate-node-${gateId}`).locator('.gt')).toHaveText('Human review')
      await expect(page.getByTestId(`gate-node-${gateId}`).locator('.gs')).toContainText('Draft is ready to polish')
      await expect(page.locator('.fmx .world .wires path.wire')).toHaveCount(connectionCount)

      await page.getByTestId(`run-mission-${missionId}`).click()
      await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('blocked')
      await expect(page.getByTestId(`slot-${draftId}-${draftSlotId}`)).toHaveClass(/active done/)
      await expect(page.getByTestId(`gate-node-${gateId}`)).toHaveClass(/\b(done|blocked)\b/)
      await expect(page.getByTestId(`formation-node-${polishId}`).locator('.fstatus')).toHaveText('')
    } finally {
      fixture?.cleanup()
    }
  })

  test('approves a human gate, resumes, and reloads final success from Archon ledger state', async ({ page }) => {
    let fixture: ReturnType<typeof createArchonPoemRoundTripFixture> | null = null
    try {
      fixture = createArchonPoemRoundTripFixture()
      const missionId = requiredString(fixture.mission.id, 'mission id')
      const polishId = requiredString(fixture.polish.id, 'polish formation id')
      const polishSlotId = requiredString(requiredArray(fixture.polish.slots, 'polish slots')[0]?.id, 'polish slot id')
      const { runId, gateId, boardSlug } = await installArchonRunLifecycleHarness(page, fixture)

      await page.goto('/')
      await page.getByRole('button', { name: 'Formations' }).click()
      await expect(page.getByTestId('formations-view')).toBeVisible()

      await page.getByTestId(`run-mission-${missionId}`).click()
      await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('running')
      await expect(page.getByRole('button', { name: `Approve gate ${gateId}` })).toBeVisible()

      await page.getByRole('button', { name: `Approve gate ${gateId}` }).click()
      await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('blocked')
      await expect(page.getByRole('button', { name: 'Resume run' })).toBeVisible()

      await page.getByRole('button', { name: 'Resume run' }).click()
      await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('succeeded')
      await expect(page.getByTestId(`slot-${polishId}-${polishSlotId}`)).toHaveClass(/active done/)

      await page.evaluate(({ key, value }) => {
        window.localStorage.setItem(key, value)
      }, { key: activeRunStorageKey(boardSlug), value: runId })
      await page.reload()
      await page.getByRole('button', { name: 'Formations' }).click()
      await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('succeeded')
      await expect(page.getByTestId(`slot-${polishId}-${polishSlotId}`)).toHaveClass(/active done/)
    } finally {
      fixture?.cleanup()
    }
  })

  test('reloads a blocked run and reconciles external board changes without losing run context', async ({ page }) => {
    const { runId, boardSlug } = await installReloadRecoveryHarness(page)
    await page.addInitScript(({ key, value }) => {
      window.localStorage.setItem(key, value)
    }, { key: activeRunStorageKey(boardSlug), value: runId })

    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toBeVisible()

    await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('blocked')
    await expect(page.getByRole('button', { name: 'Resume run' })).toBeVisible()
    await expect(page.getByTestId('formation-node-formation-external-polish')).toBeVisible({ timeout: 3000 })
    await expect(page.locator('.fmx .boardpick .rev')).toHaveText('rev 4')
    await expect(page.getByTestId('run-banner').locator('.badge')).toHaveText('blocked')
    await expect(page.getByRole('button', { name: `Approve gate gate-review` })).toBeVisible()
  })
})

/**
 * Direct-manipulation gestures: each must round-trip through the same Go board-writer
 * patch the CLI/API use. The mock board does not mutate, so we capture the PATCH body
 * and assert the gesture produced the correct patch shape (assignSlot / wireConnection /
 * createGate) — proving the cockpit drives the model, not a UI-only illusion.
 */
test.describe('Formations cockpit — direct manipulation gestures', () => {
  let patches: Record<string, unknown>[]
  let layoutPatches: Record<string, unknown>[]

  test.beforeEach(async ({ page }) => {
    patches = []
    layoutPatches = []
    const shipFormation = {
      id: 'formation-ship',
      type: 'solo' as const,
      title: 'Ship formation',
      inputs: [{ id: 'in', label: 'input' }],
      outputs: [{ id: 'out', label: 'output' }],
      slots: [{ id: 'slot-shipper', label: 'Shipper', controller: false }],
    }
    const board = {
      ...mockFormationsBoard,
      formations: [...mockFormationsBoard.formations, shipFormation],
    }
    const layout = {
      ...mockFormationsLayout,
      nodes: [...mockFormationsLayout.nodes, { id: shipFormation.id, x: 1040, y: 130 }],
    }
    await mockApiRoutes(page)
    await mockFormationsApiRoutes(page, { board, layout })
    // Capture board PATCHes (registered last → takes precedence over the mock catch-all).
    await page.route(`**/api/formations/boards/${board.slug}`, async route => {
      if (route.request().method() === 'PATCH') {
        patches.push(route.request().postDataJSON() as Record<string, unknown>)
        await route.fulfill({ status: 200, headers: { etag: board.etag }, contentType: 'application/json', body: apiBody({ board, layout: null }) })
        return
      }
      await route.fallback()
    })
    await page.route(`**/api/formations/boards/${board.slug}/layout`, async route => {
      if (route.request().method() === 'PATCH') {
        layoutPatches.push(route.request().postDataJSON() as Record<string, unknown>)
        await route.fulfill({ status: 200, headers: { etag: layout.etag }, contentType: 'application/json', body: apiBody({ layout }) })
        return
      }
      await route.fallback()
    })
    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toBeVisible()
  })

  async function pointerDrag(page: import('@playwright/test').Page, from: import('@playwright/test').Locator, toX: number, toY: number) {
    const a = await from.boundingBox()
    if (!a) throw new Error('drag source has no box')
    await pointerDragFromPoint(page, a.x + a.width / 2, a.y + a.height / 2, toX, toY)
  }

  async function pointerDragFromPoint(page: import('@playwright/test').Page, startX: number, startY: number, toX: number, toY: number) {
    await page.mouse.move(startX, startY)
    await page.mouse.down()
    await page.mouse.move(startX + 6, startY + 6)
    await page.mouse.move(toX, toY, { steps: 10 })
    await page.mouse.up()
  }

  async function pointerCancel(page: import('@playwright/test').Page, clientX: number, clientY: number) {
    await page.evaluate(({ clientX, clientY }) => {
      window.dispatchEvent(new PointerEvent('pointercancel', { bubbles: true, pointerId: 1, clientX, clientY }))
    }, { clientX, clientY })
    // Release Playwright's physical mouse state. The pointerup must be inert
    // because cancellation has already relinquished interaction ownership.
    await page.mouse.up()
  }

  async function visibleSvgPathPoint(path: import('@playwright/test').Locator) {
    return path.evaluate((node: SVGPathElement) => {
      const total = node.getTotalLength()
      const matrix = node.getScreenCTM()
      if (!matrix) throw new Error('wire path has no screen matrix')
      // Middle-out: points near a wire's ends fall into the 70px reconnect
      // zones (beginWireDrag), and this helper's callers want the lane gesture.
      for (const fraction of [0.5, 0.4, 0.6, 0.35, 0.65, 0.25, 0.75]) {
        const point = node.getPointAtLength(total * fraction)
        const screen = new DOMPoint(point.x, point.y).matrixTransform(matrix)
        const hit = document.elementFromPoint(screen.x, screen.y)
        if (hit === node || hit?.classList?.contains('wire') || hit?.classList?.contains('wirehit')) {
          return { x: screen.x, y: screen.y }
        }
      }
      throw new Error('wire path has no visible hit point')
    })
  }

  function layoutNodePatch(id: string) {
    for (const patch of layoutPatches) {
      const nodes = patch.nodes
      if (!Array.isArray(nodes)) continue
      const node = nodes.find((item): item is { id: string; x: number; y: number } => {
        if (!item || typeof item !== 'object') return false
        const candidate = item as Record<string, unknown>
        return candidate.id === id && typeof candidate.x === 'number' && typeof candidate.y === 'number'
      })
      if (node) return node
    }
    return null
  }

  async function nodeStylePosition(locator: Locator) {
    return locator.evaluate(element => {
      const node = element as HTMLElement
      return { x: Number.parseFloat(node.style.left), y: Number.parseFloat(node.style.top) }
    })
  }

  test('drag-to-staff: dragging a roster agent onto a slot emits assignSlot', async ({ page }) => {
    const formation = mockFormationsBoard.formations[0]
    const slot = page.getByTestId(`slot-${formation.id}-slot-reviewer`)
    const box = await slot.boundingBox()
    if (!box) throw new Error('no slot box')
    await pointerDrag(page, page.getByTestId('roster-agent-codex'), box.x + box.width / 2, box.y + box.height / 2)
    await expect.poll(() => patches.find(p => 'assignSlot' in p)).toBeTruthy()
    const patch = patches.find(p => 'assignSlot' in p)?.assignSlot as { formationId: string; slotId: string; agentId: string }
    expect(patch).toMatchObject({ formationId: formation.id, slotId: 'slot-reviewer', agentId: 'codex' })
  })

  test('port-drag wiring: dragging an output port to an input port emits wireConnection', async ({ page }) => {
    const input = page.locator('[data-port-in="formation-review:in"]')
    const box = await input.boundingBox()
    if (!box) throw new Error('no input port box')
    await pointerDrag(page, page.locator('[data-port-out="mission-smoke:out"]'), box.x + box.width / 2, box.y + box.height / 2)
    await expect.poll(() => patches.find(p => 'wireConnection' in p)).toBeTruthy()
    expect(patches.find(p => 'wireConnection' in p)?.wireConnection).toMatchObject({ from: 'mission-smoke:out', to: 'formation-review:in' })
  })

  test('gate token drag-to-create: dropping the Gate token on the canvas emits createGate', async ({ page }) => {
    const viewport = page.locator('.fmx .viewport')
    const vbox = await viewport.boundingBox()
    if (!vbox) throw new Error('no viewport box')
    await pointerDrag(page, page.getByTestId('gate-token'), vbox.x + vbox.width * 0.5, vbox.y + vbox.height * 0.6)
    await expect.poll(() => patches.find(p => 'createGate' in p)).toBeTruthy()
  })

  test('canvas Mission form submits the authored fields and a required Bead ID', async ({ page }) => {
    const viewport = page.locator('.fmx .viewport')
    const vbox = await requiredBox(viewport, 'formations viewport')
    await viewport.evaluate((element, point) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: point.clientX,
        clientY: point.clientY,
      }))
    }, {
      clientX: vbox.x + vbox.width * 0.7,
      clientY: vbox.y + vbox.height * 0.7,
    })
    await page.getByRole('menuitem', { name: 'Mission' }).click()

    await expect(page.getByRole('dialog', { name: 'Create mission' })).toBeVisible()
    expect(patches.filter(patch => 'createMission' in patch)).toEqual([])
    await page.getByLabel('Mission title').fill('Plan release')
    await page.getByLabel('Mission goal').fill('Ship the reduced candidate')
    await page.getByLabel('Mission Bead ID').fill('home-vdki.34.1')
    await page.getByRole('button', { name: 'Create mission' }).click()

    await expect.poll(() => patches.find(patch => 'createMission' in patch)).toBeTruthy()
    const create = patches.find(patch => 'createMission' in patch)?.createMission as Record<string, unknown>
    expect(create).toMatchObject({
      title: 'Plan release',
      goal: 'Ship the reduced candidate',
      beadId: 'home-vdki.34.1',
    })
    expect(create.x).toEqual(expect.any(Number))
    expect(create.y).toEqual(expect.any(Number))
    await expect(page.getByRole('dialog', { name: 'Create mission' })).toHaveCount(0)
  })

  test('node-drag: moving mission, formation, and gate cards persists layout node patches', async ({ page }) => {
    const mission = page.getByTestId('mission-node-mission-smoke')
    const missionStart = await nodeStylePosition(mission)
    const missionHandle = page.getByTestId('mission-node-mission-smoke').locator('.mtitle')
    const missionBox = await requiredBox(missionHandle, 'mission drag handle')
    await pointerDragFromPoint(page, missionBox.centerX, missionBox.centerY, missionBox.centerX + 84, missionBox.centerY + 36)
    await expect.poll(() => layoutNodePatch('mission-smoke')?.id).toBe('mission-smoke')
    const missionPatch = layoutNodePatch('mission-smoke')
    expect(missionPatch?.x).toBeGreaterThan(missionStart.x)
    expect(missionPatch?.y).toBeGreaterThan(missionStart.y)

    const formation = page.getByTestId('formation-node-formation-review')
    const formationStart = await nodeStylePosition(formation)
    const formationHandle = page.getByTestId('formation-node-formation-review').locator('.fhead')
    const formationBox = await requiredBox(formationHandle, 'formation drag handle')
    await pointerDragFromPoint(page, formationBox.centerX, formationBox.centerY, formationBox.centerX + 72, formationBox.centerY - 48)
    await expect.poll(() => layoutNodePatch('formation-review')?.id).toBe('formation-review')
    const formationPatch = layoutNodePatch('formation-review')
    expect(formationPatch?.x).toBeGreaterThan(formationStart.x)
    expect(formationPatch?.y).toBeLessThan(formationStart.y)

    const gate = page.getByTestId('gate-node-gate-review')
    const gateStart = await nodeStylePosition(gate)
    const gateHandle = page.getByTestId('gate-node-gate-review').locator('.gico')
    const gateBox = await requiredBox(gateHandle, 'gate drag handle')
    await pointerDragFromPoint(page, gateBox.centerX, gateBox.centerY, gateBox.centerX - 68, gateBox.centerY + 44)
    await expect.poll(() => layoutNodePatch('gate-review')?.id).toBe('gate-review')
    const gatePatch = layoutNodePatch('gate-review')
    expect(gatePatch?.x).toBeLessThan(gateStart.x)
    expect(gatePatch?.y).toBeGreaterThan(gateStart.y)
  })

  test('releases keyboard shortcuts while the keep-alive cockpit is hidden and restores them on return', async ({ page }) => {
    const mission = page.getByTestId('mission-node-mission-smoke')
    const missionHandle = mission.locator('.mtitle')
    const missionBox = await requiredBox(missionHandle, 'mission drag handle')
    await pointerDragFromPoint(page, missionBox.centerX, missionBox.centerY, missionBox.centerX + 84, missionBox.centerY + 36)
    await expect.poll(() => layoutPatches.length).toBe(1)

    const viewport = page.locator('.fmx .viewport')
    const viewportBox = await requiredBox(viewport, 'formations viewport')
    await viewport.evaluate((element, point) => {
      element.dispatchEvent(new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        button: 2,
        clientX: point.clientX,
        clientY: point.clientY,
      }))
    }, {
      clientX: viewportBox.x + viewportBox.width * 0.7,
      clientY: viewportBox.y + viewportBox.height * 0.7,
    })
    await page.getByRole('menuitem', { name: 'Mission' }).click()
    await expect(page.getByRole('dialog', { name: 'Create mission' })).toBeVisible()

    await page.getByRole('button', { name: 'Settings' }).click()
    await expect(page.getByTestId('formations-host')).toBeHidden()

    const hiddenShortcutOwnership = await page.evaluate(() => {
      const undo = new KeyboardEvent('keydown', { key: 'z', ctrlKey: true, bubbles: true, cancelable: true })
      window.dispatchEvent(undo)
      const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
      window.dispatchEvent(escape)
      return { undoPrevented: undo.defaultPrevented, escapePrevented: escape.defaultPrevented }
    })
    expect(hiddenShortcutOwnership).toEqual({ undoPrevented: false, escapePrevented: false })

    await page.getByRole('button', { name: 'Formations' }).click()
    await expect(page.getByTestId('formations-view')).toBeVisible()
    expect(layoutPatches).toHaveLength(1)
    await expect(page.getByRole('dialog', { name: 'Create mission' })).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog', { name: 'Create mission' })).toHaveCount(0)

    await page.keyboard.press('Control+z')
    await expect.poll(() => layoutPatches.length).toBe(2)
  })

  test('pointercancel clears every gesture projection without committing a drop', async ({ page }) => {
    const viewport = page.locator('.fmx .viewport')
    const viewportBox = await requiredBox(viewport, 'formations viewport')

    // Pan: cancellation relinquishes the viewport class.
    const panStart = { x: viewportBox.x + viewportBox.width - 80, y: viewportBox.y + viewportBox.height - 80 }
    await page.mouse.move(panStart.x, panStart.y)
    await page.mouse.down()
    await page.mouse.move(panStart.x + 32, panStart.y + 24)
    await expect(viewport).toHaveClass(/panning/)
    await pointerCancel(page, panStart.x + 32, panStart.y + 24)
    await expect(viewport).not.toHaveClass(/panning/)

    // Node: the transient position and drag class disappear without a layout PATCH.
    const node = page.getByTestId('mission-node-mission-smoke')
    const nodeStart = await nodeStylePosition(node)
    const nodeHandle = await requiredBox(node.locator('.mtitle'), 'mission drag handle')
    await page.mouse.move(nodeHandle.centerX, nodeHandle.centerY)
    await page.mouse.down()
    await page.mouse.move(nodeHandle.centerX + 70, nodeHandle.centerY + 36)
    await expect(page.getByTestId('formations-world')).toHaveClass(/nodedrag/)
    expect(await nodeStylePosition(node)).not.toEqual(nodeStart)
    await pointerCancel(page, nodeHandle.centerX + 70, nodeHandle.centerY + 36)
    await expect(page.getByTestId('formations-world')).not.toHaveClass(/nodedrag/)
    expect(await nodeStylePosition(node)).toEqual(nodeStart)
    expect(layoutPatches).toEqual([])

    // Reconnect: the committed wire returns and temp/hover projections clear.
    const connectedInput = page.locator('[data-port-in="gate-review:in"]')
    const connectedInputBox = await requiredBox(connectedInput, 'connected gate input')
    const reconnectTarget = page.locator('[data-port-in="formation-ship:in"]')
    const reconnectTargetBox = await requiredBox(reconnectTarget, 'reconnect target')
    await page.mouse.move(connectedInputBox.centerX, connectedInputBox.centerY)
    await page.mouse.down()
    await page.mouse.move(reconnectTargetBox.centerX, reconnectTargetBox.centerY)
    await expect(page.locator('.fmx path.wire.temp')).toHaveCount(1)
    await expect(page.getByTestId('formation-wire-conn-review-gate')).toHaveCount(0)
    await expect(reconnectTarget).toHaveClass(/snaptarget/)
    await pointerCancel(page, reconnectTargetBox.centerX, reconnectTargetBox.centerY)
    await expect(page.locator('.fmx path.wire.temp')).toHaveCount(0)
    await expect(page.getByTestId('formation-wire-conn-review-gate')).toHaveCount(1)
    await expect(reconnectTarget).not.toHaveClass(/snaptarget/)
    expect(patches).toEqual([])

    // Judge-wire hover is owned by the same cancellable wire projection.
    const judgeSocket = page.getByTestId('gate-judge-socket-gate-review')
    const judgeSocketBox = await requiredBox(judgeSocket, 'judge socket')
    const judgeTarget = page.getByTestId('formation-node-formation-ship')
    const judgeTargetBox = await requiredBox(judgeTarget, 'judge formation target')
    await page.mouse.move(judgeSocketBox.centerX, judgeSocketBox.centerY)
    await page.mouse.down()
    await page.mouse.move(judgeTargetBox.centerX, judgeTargetBox.centerY)
    await expect(judgeTarget).toHaveClass(/judgehover/)
    await pointerCancel(page, judgeTargetBox.centerX, judgeTargetBox.centerY)
    await expect(page.locator('.fmx path.wire.temp')).toHaveCount(0)
    await expect(judgeTarget).not.toHaveClass(/judgehover/)
    expect(patches).toEqual([])

    // Gate token: its screen-space ghost clears without createGate.
    const gateTokenBox = await requiredBox(page.getByTestId('gate-token'), 'gate token')
    const gateTarget = { x: viewportBox.x + viewportBox.width * 0.5, y: viewportBox.y + viewportBox.height * 0.7 }
    await page.mouse.move(gateTokenBox.centerX, gateTokenBox.centerY)
    await page.mouse.down()
    await page.mouse.move(gateTarget.x, gateTarget.y)
    await expect(page.locator('.fmx .gateghost')).toHaveCount(1)
    await pointerCancel(page, gateTarget.x, gateTarget.y)
    await expect(page.locator('.fmx .gateghost')).toHaveCount(0)
    expect(patches).toEqual([])

    // Lane: the draft route reverts without persisting an edge lane.
    const wire = page.getByTestId('formation-wire-conn-review-gate')
    const originalPath = await wire.getAttribute('d')
    const wirePoint = await visibleSvgPathPoint(wire)
    await page.mouse.move(wirePoint.x, wirePoint.y)
    await page.mouse.down()
    await page.mouse.move(wirePoint.x, wirePoint.y + 90)
    await expect.poll(() => wire.getAttribute('d')).not.toBe(originalPath)
    await pointerCancel(page, wirePoint.x, wirePoint.y + 90)
    await expect(wire).toHaveAttribute('d', originalPath || '')
    expect(layoutPatches).toEqual([])

    // Staff: the ghost and slot hover clear without assigning the agent.
    const slot = page.getByTestId('slot-formation-review-slot-reviewer')
    const slotBox = await requiredBox(slot, 'staff target slot')
    const rosterAgentBox = await requiredBox(page.getByTestId('roster-agent-codex'), 'roster agent')
    await page.mouse.move(rosterAgentBox.centerX, rosterAgentBox.centerY)
    await page.mouse.down()
    await page.mouse.move(slotBox.centerX, slotBox.centerY)
    await expect(page.locator('.fmx-ghost')).toHaveCount(1)
    await expect(slot).toHaveClass(/snaptarget/)
    await pointerCancel(page, slotBox.centerX, slotBox.centerY)
    await expect(page.locator('.fmx-ghost')).toHaveCount(0)
    await expect(slot).not.toHaveClass(/snaptarget/)
    expect(patches).toEqual([])
  })

  test('Arrange invokes the shared layout operation and undoes in one step', async ({ page }) => {
    await page.getByTestId('arrange-layout').click()
    await expect.poll(() => layoutPatches.find(p => p.arrange === true)).toBeTruthy()

    const patchesBefore = layoutPatches.length
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
    await expect.poll(() => layoutPatches.length).toBeGreaterThan(patchesBefore)
    const undoPatch = layoutPatches[layoutPatches.length - 1]
    expect(Array.isArray(undoPatch.nodes)).toBe(true)
    expect((undoPatch.nodes as unknown[]).length).toBe(mockFormationsLayout.nodes.length + 1)
  })

  test('local formation menu edits input through a popover and undo emits the inverse model operation', async ({ page }) => {
    const formation = { id: 'formation-ship', title: 'Ship formation' }
    // Right-click the card header (not the slot, which has its own menu per the reference).
    await page.getByTestId(`formation-node-${formation.id}`).locator('.fhead').click({ button: 'right' })
    await expect(page.getByRole('menu', { name: 'Formation actions' })).toBeVisible()
    await page.getByRole('menuitem', { name: 'Set input' }).click()

    await expect(page.getByRole('dialog', { name: `Input · ${formation.title}` })).toBeVisible()
    await page.getByLabel(`Goal for ${formation.title}`).fill('Reconcile the UI reference blockers')
    await page.getByLabel(`Bead for ${formation.title}`).fill('home-vdki.31')
    await page.getByRole('button', { name: 'Save input' }).click()

    await expect.poll(() => patches.find(p => 'setBrief' in p)).toBeTruthy()
    expect(patches.find(p => 'setBrief' in p)?.setBrief).toMatchObject({
      formationId: formation.id,
      goal: 'Reconcile the UI reference blockers',
      beadId: 'home-vdki.31',
    })

    await page.locator('.fmx .viewport').click({ position: { x: 20, y: 20 } })
    await page.keyboard.press(process.platform === 'darwin' ? 'Meta+Z' : 'Control+Z')
    await expect.poll(() => patches.find(p => 'clearBrief' in p)).toBeTruthy()
    expect(patches.find(p => 'clearBrief' in p)?.clearBrief).toMatchObject({ formationId: formation.id })
  })

  test('wire gestures reconnect, reroute, reset, and remove through board/layout operations', async ({ page }) => {
    await page.getByRole('button', { name: 'FIT' }).click()
    await expect(page.locator('.fmx .zoomlevel')).not.toHaveText('100%')
    const shipInput = page.locator('[data-port-in="formation-ship:in"]')
    const shipBox = await shipInput.boundingBox()
    if (!shipBox) throw new Error('ship input has no box')
    await pointerDrag(page, page.locator('[data-port-in="gate-review:in"]'), shipBox.x + shipBox.width / 2, shipBox.y + shipBox.height / 2)
    await expect.poll(() => patches.find(p => 'rewireConnection' in p)).toBeTruthy()
    expect(patches.find(p => 'rewireConnection' in p)?.rewireConnection).toMatchObject({
      from: 'formation-review:out',
      previousTo: 'gate-review:in',
      to: 'formation-ship:in',
    })

    const wire = page.locator('.fmx .wires path.wire').first()
    const wirePoint = await visibleSvgPathPoint(wire)
    // Hand-routing the MIDDLE of a wire sets a lane POINT (reference `via.y`), not an inert string.
    await pointerDragFromPoint(page, wirePoint.x, wirePoint.y, wirePoint.x, wirePoint.y + 92)
    await expect.poll(() => layoutPatches.find(p => 'edges' in p)).toBeTruthy()
    const laneEdge = (layoutPatches.find(p => 'edges' in p)?.edges as { id: string; lane: string }[])[0]
    expect(laneEdge.id).toBe('conn-review-gate')
    expect(laneEdge.lane).toMatch(/^y:-?\d+$/)

    await page.mouse.click(wirePoint.x, wirePoint.y, { button: 'right' })
    await page.getByRole('menuitem', { name: 'Reset routing' }).click()
    await expect.poll(() => layoutPatches.some(p => JSON.stringify(p).includes('"lane":"auto"'))).toBe(true)

    await page.mouse.click(wirePoint.x, wirePoint.y, { button: 'right' })
    await page.getByRole('menuitem', { name: 'Remove connection' }).click()
    await expect.poll(() => patches.find(p => 'unwireConnection' in p)).toBeTruthy()
    expect(patches.find(p => 'unwireConnection' in p)?.unwireConnection).toMatchObject({
      from: 'formation-review:out',
      to: 'gate-review:in',
    })
  })

  test('gate judge socket opens local affordances and attaches an existing formation as judge', async ({ page }) => {
    // A plain click on the judge socket opens the reference judge picker (new-judge
    // options plus existing formations by title and detach when wired).
    await expect(page.getByTestId('gate-judge-socket-gate-review')).toBeVisible()
    await page.getByTestId('gate-judge-socket-gate-review').click()
    await expect(page.getByRole('menu', { name: 'Judge' })).toBeVisible()
    await expect(page.getByRole('menuitem', { name: 'Solo · 1 agent' })).toBeVisible()
    await page.getByRole('menuitem', { name: 'Review formation' }).click()

    await expect.poll(() => patches.find(p => 'setGateJudge' in p)).toBeTruthy()
    expect(patches.find(p => 'setGateJudge' in p)?.setGateJudge).toMatchObject({
      gateId: 'gate-review',
      chain: ['formation-review'],
    })
  })

  test('gate judge socket attaches a formation by dragging the judge wire onto it', async ({ page }) => {
    const reviewCard = page.getByTestId('formation-node-formation-review')
    const box = await reviewCard.boundingBox()
    if (!box) throw new Error('review formation has no box')
    await pointerDrag(page, page.getByTestId('gate-judge-socket-gate-review'), box.x + box.width / 2, box.y + box.height / 2)
    await expect.poll(() => patches.find(p => 'setGateJudge' in p)).toBeTruthy()
    expect(patches.find(p => 'setGateJudge' in p)?.setGateJudge).toMatchObject({
      gateId: 'gate-review',
      chain: ['formation-review'],
    })
  })
})
