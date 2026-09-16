import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { BeadProject, BeadRow, BeadWork } from '../beads/beadsApi'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { DEFAULT_SETTINGS } from '../types'
import BeadsColumn, { arrangeBeadsColumnGroups } from './BeadsColumn'

const mockState = vi.hoisted(() => ({
  updateSettings: vi.fn(),
  projects: [] as BeadProject[],
  manualProjects: [] as BeadProject[],
  beadsProjectPaths: [] as string[],
  workRequests: [] as string[],
  work: new Map<string, BeadWork | Error>(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: { ...DEFAULT_SETTINGS, beadsProjectPaths: mockState.beadsProjectPaths },
    updateSettings: mockState.updateSettings,
  }),
}))

vi.mock('../beads/beadsApi', async () => {
  const actual = await vi.importActual<typeof import('../beads/beadsApi')>('../beads/beadsApi')
  return {
    ...actual,
    fetchBeadProjectList: () => Promise.resolve(mockState.projects),
    fetchManualBeadProjects: () => Promise.resolve(mockState.manualProjects),
    fetchBeadWork: (path: string) => {
      mockState.workRequests.push(path)
      const result = mockState.work.get(path)
      return result instanceof Error ? Promise.reject(result) : Promise.resolve(result)
    },
  }
})

function row(id: string, status: string, updated: string, blocked = false): BeadRow {
  return { id, title: `Title ${id}`, status, priority: 1, updated, blocked, linked: false }
}

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request?.id ?? 'none'}</span>
}

beforeEach(() => {
  mockState.updateSettings.mockReset()
  mockState.manualProjects = []
  mockState.beadsProjectPaths = []
  mockState.workRequests = []
  mockState.projects = [
    { name: 'zeta', path: '/zeta', beadsPath: '/zeta/.beads', prefix: 'z', openBeads: 3 },
    { name: 'alpha', path: '/alpha', beadsPath: '/alpha/.beads', prefix: 'a', openBeads: 2 },
    { name: 'quiet', path: '/quiet', beadsPath: '/quiet/.beads', prefix: 'q', counts: {
      status: { open: 0, inProgress: 0, blocked: 1, closed: 2, deferred: 0 },
      type: { epic: 0, task: 3, bug: 0, feature: 0, decision: 0, chore: 0 },
    } },
  ]
  mockState.work = new Map([
    ['/zeta', { prefix: 'z', projectPath: '/zeta', beads: [
      row('z-old-ready', 'open', '2026-09-01T00:00:00Z'),
      row('z-new-active', 'in_progress', '2026-09-04T00:00:00Z'),
      row('z-old-active', 'in_progress', '2026-09-02T00:00:00Z'),
      row('z-new-ready', 'open', '2026-09-03T00:00:00Z'),
    ] }],
    ['/alpha', { prefix: 'a', projectPath: '/alpha', beads: [
      row('a-ready', 'open', '2026-09-04T00:00:00Z'),
      row('a-blocked', 'open', '2026-09-05T00:00:00Z', true),
    ] }],
  ])
})

afterEach(() => resetBeadCardForTest())

describe('the Beads column', () => {
  it('groups stores, puts in-progress before ready, and orders each newest first', () => {
    const groups = arrangeBeadsColumnGroups([
      { project: mockState.projects[0], work: mockState.work.get('/zeta') as BeadWork },
      { project: mockState.projects[1], work: mockState.work.get('/alpha') as BeadWork },
    ])

    expect(groups.map(group => group.label)).toEqual(['a', 'z'])
    expect(groups[1].inProgress.map(item => item.id)).toEqual(['z-new-active', 'z-old-active'])
    expect(groups[1].ready.map(item => item.id)).toEqual(['z-new-ready', 'z-old-ready'])
    expect(groups[0].ready.map(item => item.id)).toEqual(['a-ready'])
  })

  it('opens a row on the table and leaves proven-quiet stores unread', async () => {
    render(<><BeadsColumn open onClose={vi.fn()} /><CardProbe /></>)

    const alpha = await screen.findByRole('heading', { name: 'a' })
    expect(screen.queryByRole('heading', { name: 'q' })).toBeNull()
    expect(mockState.work.has('/quiet')).toBe(false)

    fireEvent.keyDown(screen.getByRole('separator', { name: 'Resize Beads column' }), { key: 'ArrowLeft' })
    expect(mockState.updateSettings).toHaveBeenCalledWith({ beadsColumnWidth: 376 })

    fireEvent.click(within(alpha.parentElement as HTMLElement).getByRole('button', { name: /a-ready/ }))
    await waitFor(() => expect(screen.getByTestId('card-request')).toHaveTextContent('a-ready'))
  })

  it('reads each store once when a manual path repeats a listed store', async () => {
    mockState.beadsProjectPaths = ['/zeta', '/manual']
    mockState.manualProjects = [
      { name: 'zeta', path: '/zeta', beadsPath: '/zeta/.beads', prefix: 'z', openBeads: 3 },
      { name: 'manual', path: '/manual', beadsPath: '/manual/.beads', prefix: 'm', openBeads: 1 },
    ]
    mockState.work.set('/manual', { prefix: 'm', projectPath: '/manual', beads: [
      row('m-ready', 'open', '2026-09-04T00:00:00Z'),
    ] })

    render(<BeadsColumn open onClose={vi.fn()} />)

    expect(await screen.findByText('m-ready')).toBeInTheDocument()
    expect(mockState.workRequests.filter(path => path === '/zeta')).toHaveLength(1)
    expect([...mockState.workRequests].sort()).toEqual(['/alpha', '/manual', '/zeta'])
  })

  it('lists a failed store without hiding readable work', async () => {
    mockState.work.set('/alpha', new Error('permission denied'))
    render(<BeadsColumn open onClose={vi.fn()} />)

    expect(await screen.findByText('z-new-active')).toBeInTheDocument()
    expect(screen.getByText('permission denied')).toBeInTheDocument()
    expect(screen.getByText('a')).toBeInTheDocument()
  })
})
