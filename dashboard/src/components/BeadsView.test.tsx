import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BeadsView from './BeadsView'
import { DEFAULT_SETTINGS } from '../types'
import { resetBeadCardForTest, useBeadCardRequest } from '../beads/beadCard'
import { resetBeadProjectsForTest } from '../beads/beadIds'
import type { BeadRow } from '../beads/beadsApi'

const mockState = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
  announce: vi.fn(),
  updateSettings: vi.fn(),
  settings: null as unknown as typeof DEFAULT_SETTINGS,
  projectList: null as unknown[] | null,
  projects: [] as unknown[],
  work: new Map<string, unknown>(),
  closed: new Map<string, unknown>(),
  formulas: new Map<string, unknown>(),
  molecules: new Map<string, unknown>(),
  formulaDetails: new Map<string, unknown>(),
  moleculeDetails: new Map<string, unknown>(),
  beadDetails: new Map<string, unknown>(),
  fetchBead: vi.fn(),
  fetchBeadWork: vi.fn(),
  fetchClosedBeadWork: vi.fn(),
  fetchFormulas: vi.fn(),
  fetchMolecules: vi.fn(),
  fetchFormula: vi.fn(),
  fetchMolecule: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: mockState.settings,
    updateSettings: mockState.updateSettings,
    openSendToSession: mockState.openSendToSession,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../beads/beadsApi', () => ({
  fetchBeadProjectList: () => Promise.resolve(mockState.projectList ?? mockState.projects),
  fetchBeadProjects: () => Promise.resolve(mockState.projects),
  fetchBeadWork: (path: string) => mockState.fetchBeadWork(path),
  fetchClosedBeadWork: (path: string) => mockState.fetchClosedBeadWork(path),
  fetchFormulas: (path: string) => mockState.fetchFormulas(path),
  fetchMolecules: (path: string) => mockState.fetchMolecules(path),
  fetchFormula: (path: string, name: string) => mockState.fetchFormula(path, name),
  fetchMolecule: (path: string, id: string) => mockState.fetchMolecule(path, id),
  fetchBead: (path: string, id: string) => mockState.fetchBead(path, id),
}))

vi.mock('./ResidentColumn', () => ({
  default: ({ reference }: { reference: string | null }) => (
    <div data-testid="resident" data-reference={reference ?? ''} />
  ),
}))

function bead(overrides: Partial<BeadRow> & { id: string }): BeadRow {
  return { title: `Title of ${overrides.id}`, status: 'open', priority: 1, blocked: false, linked: false, ...overrides }
}

const OLD = new Date(Date.now() - 40 * 86400000).toISOString()
const FRESH = new Date(Date.now() - 1 * 86400000).toISOString()

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request?.id ?? 'none'}</span>
}

beforeEach(() => {
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.updateSettings.mockReset()
  mockState.settings = DEFAULT_SETTINGS
  mockState.projectList = null
  mockState.projects = [
    { name: 'chrote', path: '/srv/chrote', beadsPath: '/srv/chrote/.beads', prefix: 'chrote', openBeads: 3 },
    { name: 'srv', path: '/srv', beadsPath: '/srv/.beads', prefix: 'ctx', openBeads: 1 },
    { name: 'quiet', path: '/srv/quiet', beadsPath: '/srv/quiet/.beads', prefix: 'qt', openBeads: 0 },
    { name: 'silent', path: '/srv/silent', beadsPath: '/srv/silent/.beads', prefix: 'sl', openBeads: 0 },
  ]
  mockState.work = new Map<string, unknown>([
    ['/srv/chrote', {
      prefix: 'chrote',
      projectPath: '/srv/chrote',
      beads: [
        bead({ id: 'chrote-ep', type: 'epic', acceptance: 'Everything under it lands', updated: FRESH, linked: true }),
        bead({ id: 'chrote-ep.1', parent: 'chrote-ep', type: 'task', updated: FRESH, linked: true }),
        bead({
          id: 'chrote-ep.2',
          parent: 'chrote-ep',
          type: 'task',
          priority: 2,
          blocked: true,
          blockedBy: ['chrote-ep.1'],
          linked: true,
          updated: OLD,
        }),
      ],
    }],
    ['/srv', { prefix: 'ctx', projectPath: '/srv', beads: [bead({ id: 'ctx-t4ak', type: 'task', updated: OLD })] }],
    ['/srv/quiet', { prefix: 'qt', projectPath: '/srv/quiet', beads: [] }],
    ['/srv/silent', { prefix: 'sl', projectPath: '/srv/silent', beads: [] }],
  ])
  mockState.closed = new Map<string, unknown>([
    ['/srv/chrote', {
      prefix: 'chrote', projectPath: '/srv/chrote',
      beads: [bead({ id: 'chrote-done', title: 'Finished dashboard pass', status: 'closed', type: 'task', parent: 'chrote-ep', linked: true, updated: FRESH })],
    }],
    ['/srv', {
      prefix: 'ctx', projectPath: '/srv',
      beads: [bead({ id: 'ctx-dup', title: 'Superseded note', status: 'duplicate', type: 'task', updated: OLD })],
    }],
    ['/srv/quiet', {
      prefix: 'qt', projectPath: '/srv/quiet',
      beads: [bead({ id: 'qt-done', title: 'Quiet store history', status: 'closed', type: 'task', updated: OLD })],
    }],
    ['/srv/silent', { prefix: 'sl', projectPath: '/srv/silent', beads: [] }],
  ])
  mockState.formulas = new Map<string, unknown>([
    ['/srv/chrote', {
      projectPath: '/srv/chrote',
      formulas: [{ name: 'release', type: 'workflow', description: 'Ship the dashboard', source: '/srv/chrote/.beads/formulas/release.formula.toml' }],
    }],
  ])
  mockState.molecules = new Map<string, unknown>([
    ['/srv/chrote', {
      projectPath: '/srv/chrote',
      molecules: [
        { id: 'chrote-proto', title: 'Release template', status: 'open', is_template: true },
        { id: 'chrote-run', title: 'September release', status: 'in_progress', is_template: false, source_formula: 'release' },
      ],
    }],
  ])
  mockState.formulaDetails = new Map<string, unknown>([
    ['/srv/chrote\u0000release', {
      formula: 'release', description: 'Ship the dashboard', source: '/srv/chrote/.beads/formulas/release.formula.toml',
      vars: { target: { required: true } },
      steps: [{ id: 'build', title: 'Build' }, { id: 'ship', title: 'Ship', depends_on: ['build'] }],
    }],
  ])
  mockState.moleculeDetails = new Map<string, unknown>([
    ['/srv/chrote\u0000chrote-run', {
      root: { id: 'chrote-run', title: 'September release', status: 'in_progress', source_formula: 'release' },
      issues: [{ id: 'chrote-run.1', title: 'Build', status: 'closed' }, { id: 'chrote-run.2', title: 'Ship', status: 'open' }],
      dependencies: [{ issue_id: 'chrote-run.2', depends_on_id: 'chrote-run.1', type: 'blocks' }],
      variables: { target: 'prod' },
    }],
  ])
  mockState.beadDetails = new Map<string, unknown>([
    ['/srv/chrote\u0000chrote-ep.1', {
      id: 'chrote-ep.1', title: 'Title of chrote-ep.1', status: 'open', type: 'task', priority: 1,
      parents: [{ id: 'chrote-ep', title: 'Title of chrote-ep', status: 'open', type: 'epic', priority: 1 }],
      children: [], blockedBy: [], blocks: [],
    }],
    ['/srv/chrote\u0000chrote-done', {
      id: 'chrote-done', title: 'Finished dashboard pass', status: 'closed', type: 'task', priority: 1,
      parents: [{ id: 'chrote-ep', title: 'Title of chrote-ep', status: 'open', type: 'epic', priority: 1 }],
      children: [], blockedBy: [], blocks: [],
    }],
  ])
  mockState.fetchBeadWork.mockReset()
  mockState.fetchBeadWork.mockImplementation((path: string) => {
    const answer = mockState.work.get(path)
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
  mockState.fetchClosedBeadWork.mockReset()
  mockState.fetchClosedBeadWork.mockImplementation((path: string) => {
    const answer = mockState.closed.get(path) ?? { prefix: '', projectPath: path, beads: [] }
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
  mockState.fetchFormulas.mockReset()
  mockState.fetchFormulas.mockImplementation((path: string) => {
    const answer = mockState.formulas.get(path) ?? { projectPath: path, formulas: [] }
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
  mockState.fetchMolecules.mockReset()
  mockState.fetchMolecules.mockImplementation((path: string) => {
    const answer = mockState.molecules.get(path) ?? { projectPath: path, molecules: [] }
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
  mockState.fetchFormula.mockReset()
  mockState.fetchFormula.mockImplementation((path: string, name: string) => {
    const answer = mockState.formulaDetails.get(`${path}\u0000${name}`)
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
  mockState.fetchMolecule.mockReset()
  mockState.fetchMolecule.mockImplementation((path: string, id: string) => {
    const answer = mockState.moleculeDetails.get(`${path}\u0000${id}`)
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
  mockState.fetchBead.mockReset()
  mockState.fetchBead.mockImplementation((path: string, id: string) => {
    const answer = mockState.beadDetails.get(`${path}\u0000${id}`)
    return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer)
  })
})

afterEach(() => {
  resetBeadCardForTest()
  resetBeadProjectsForTest()
})

describe('the Beads tab', () => {
  it('draws every configured store as a map of open work', async () => {
    render(<BeadsView />)

    expect(await screen.findByText('Title of chrote-ep')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'All' })).toHaveClass('active')
    expect(screen.getByRole('button', { name: 'ctx' })).toBeInTheDocument()
    expect(screen.getByText('Acceptance criteria')).toBeInTheDocument()
    expect(screen.getByText('Everything under it lands')).toBeInTheDocument()
    expect(screen.getByText(/blocked by/)).toBeInTheDocument()
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
  })

  it('says what it loaded on the status line, and asks nothing of a quiet store', async () => {
    render(<BeadsView />)
    await waitFor(() => expect(mockState.announce).toHaveBeenCalled())
    expect(mockState.announce.mock.calls[0][0]).toBe('Beads loaded · chrote 3 open · ctx 1 open')
  })

  it('keeps readable stores visible when another store refuses the work request', async () => {
    mockState.projects.splice(2, 0, {
      name: 'broken', path: '/srv/broken', beadsPath: '/srv/broken/.beads', prefix: 'bad', openBeads: 2,
    })
    mockState.work.set('/srv/broken', new Error('permission denied'))

    render(<BeadsView />)

    expect(await screen.findByText('Title of chrote-ep')).toBeInTheDocument()
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      'Beads unavailable · bad: permission denied',
      'error',
    ))
  })

  it('lists a known unreadable store instead of loading it through All', async () => {
    mockState.projects.splice(2, 0, {
      name: 'broken', path: '/srv/broken', beadsPath: '/srv/broken/.beads', error: 'permission denied',
    })

    render(<BeadsView />)

    expect(await screen.findByRole('button', { name: 'broken · unreadable' })).toBeInTheDocument()
    expect(mockState.fetchBeadWork).not.toHaveBeenCalledWith('/srv/broken')
  })

  it('folds the stores with nothing open under one row that expands in place', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    const rail = screen.getByRole('navigation', { name: 'Beads projects' })
    const rows = () => [...rail.querySelectorAll('.beads-rail-item')].map(row => row.textContent)
    expect(rows()).toEqual(['All', 'chrote', 'ctx', 'More (2 quiet)'])

    const more = screen.getByRole('button', { name: 'More (2 quiet)' })
    expect(more).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(more)
    expect(rows()).toEqual(['All', 'chrote', 'ctx', 'Fewer', 'qt', 'sl'])
    expect(screen.getByRole('button', { name: 'Fewer' })).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'qt' }))
    await waitFor(() => expect(screen.queryByText('Title of ctx-t4ak')).toBeNull())
    expect(screen.getByRole('button', { name: 'qt' })).toHaveClass('active')

    fireEvent.click(screen.getByRole('button', { name: 'Fewer' }))
    expect(rows()).toEqual(['All', 'chrote', 'ctx', 'More (2 quiet)', 'qt'])
    expect(screen.getByRole('button', { name: 'More (2 quiet)' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('button', { name: 'qt' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'sl' })).toBeNull()
  })

  it('narrows every view by id and title', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.change(screen.getByLabelText('Search Beads'), { target: { value: 'ctx-t4ak' } })

    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
    expect(screen.queryByText('Title of chrote-ep.1')).toBeNull()
  })

  it('shows one store at a time from the rail', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('button', { name: 'ctx' }))

    await waitFor(() => expect(screen.queryByText('Title of chrote-ep')).toBeNull())
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
  })

  it('opens the card from a row, and hands the Clerk the Bead by id and title', async () => {
    render(<><BeadsView /><CardProbe /></>)
    const row = await screen.findByText('Title of chrote-ep.1')
    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', '')

    fireEvent.click(row)

    expect(screen.getByTestId('card-request')).toHaveTextContent('chrote-ep.1')
    expect(screen.getByTestId('resident')).toHaveAttribute('data-reference', 'bead chrote-ep.1: Title of chrote-ep.1')
  })

  it('hands a stale Bead over with the question already asked', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('tab', { name: 'Stale' }))

    expect(screen.getByText('Title of chrote-ep.2')).toBeInTheDocument()
    expect(screen.queryByText('Title of chrote-ep.1')).toBeNull()

    fireEvent.click(screen.getAllByRole('button', { name: 'Send' })[0])
    expect(mockState.openSendToSession).toHaveBeenCalledWith({
      reference: 'bead chrote-ep.2 looks stale: close it or revive it',
    })
  })

  it('splits ready from what is already claimed', async () => {
    mockState.work.set('/srv', {
      prefix: 'ctx',
      projectPath: '/srv',
      beads: [bead({ id: 'ctx-t4ak', type: 'task', status: 'in_progress', updated: FRESH })],
    })
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('tab', { name: 'Open' }))

    expect(screen.getByRole('heading', { name: 'Ready to start' })).toBeInTheDocument()
    const inProgress = screen.getByRole('heading', { name: 'In progress' }).parentElement as HTMLElement
    expect(inProgress).toHaveTextContent('ctx-t4ak')
    expect(inProgress).not.toHaveTextContent('chrote-ep.1')
  })

  it('restores the selected store and Open view before fetching work', async () => {
    mockState.settings = {
      ...DEFAULT_SETTINGS,
      beadsSelectedProject: '/srv',
      beadsView: 'ready',
    }

    render(<BeadsView />)

    expect(await screen.findByRole('button', { name: 'ctx' })).toHaveClass('active')
    expect(screen.getByRole('tab', { name: 'Open' })).toHaveAttribute('aria-selected', 'true')
    await waitFor(() => expect(mockState.fetchBeadWork).toHaveBeenCalled())
    expect(mockState.fetchBeadWork.mock.calls[0][0]).toBe('/srv')
    expect(mockState.fetchBeadWork).not.toHaveBeenCalledWith('/srv/chrote')
  })

  it('fills counts after the first store list without another click', async () => {
    mockState.projectList = mockState.projects.map(project => {
      const { openBeads: _openBeads, ...listed } = project as { openBeads?: number }
      return { ...listed, summaryPending: true }
    })

    render(<BeadsView />)

    expect(await screen.findByRole('button', { name: 'More (2 quiet)' })).toBeInTheDocument()
  })

  it('does not ask for closed work until Closed is selected, then caches each scope', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    expect(mockState.fetchClosedBeadWork).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('tab', { name: 'Closed' }))

    expect(await screen.findByText('Finished dashboard pass')).toBeInTheDocument()
    expect(screen.getByText('Quiet store history')).toBeInTheDocument()
    expect(mockState.fetchClosedBeadWork.mock.calls.map(call => call[0])).toEqual([
      '/srv/chrote', '/srv', '/srv/quiet', '/srv/silent',
    ])

    fireEvent.change(screen.getByLabelText('Search closed Beads'), { target: { value: 'quiet store' } })
    expect(screen.getByText('Quiet store history')).toBeInTheDocument()
    expect(screen.queryByText('Finished dashboard pass')).toBeNull()
    fireEvent.change(screen.getByLabelText('Search closed Beads'), { target: { value: 'nothing matches' } })
    expect(screen.getByText('No closed Beads match "nothing matches".')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'Map' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Closed' }))
    expect(screen.getByText('No closed Beads match "nothing matches".')).toBeInTheDocument()
    expect(mockState.fetchClosedBeadWork).toHaveBeenCalledTimes(4)

    fireEvent.change(screen.getByLabelText('Search closed Beads'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: 'ctx' }))
    expect(await screen.findByText('Superseded note')).toBeInTheDocument()
    expect(mockState.fetchClosedBeadWork.mock.calls.filter(call => call[0] === '/srv')).toHaveLength(2)
  })

  it('keeps loaded closed stores visible when another store fails', async () => {
    mockState.closed.set('/srv/quiet', new Error('permission denied'))
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')

    fireEvent.click(screen.getByRole('tab', { name: 'Closed' }))

    expect(await screen.findByText('Finished dashboard pass')).toBeInTheDocument()
    expect(screen.getByText('qt: permission denied')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Search closed Beads'), { target: { value: 'nothing' } })
    expect(screen.getByText('qt: permission denied')).toBeInTheDocument()
    expect(screen.getByText('No closed Beads match "nothing".')).toBeInTheDocument()
  })

  it('hydrates a linked row before revealing neighbours omitted from the work snapshot', async () => {
    const target = bead({ id: 'chrote-external', title: 'Cross-snapshot task', type: 'task', linked: true, updated: FRESH })
    const neighbour = { id: 'chrote-hidden', title: 'Hidden blocker', status: 'closed', type: 'task', priority: 1 }
    const work = mockState.work.get('/srv/chrote') as { beads: BeadRow[] }
    work.beads.push(target)
    mockState.beadDetails.set('/srv/chrote\u0000chrote-external', {
      ...target, parents: [], children: [], blockedBy: [neighbour], blocks: [],
    })
    render(<BeadsView />)

    fireEvent.contextMenu(await screen.findByText('Cross-snapshot task'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open in Flow' }))

    await waitFor(() => expect(mockState.fetchBead).toHaveBeenCalledWith('/srv/chrote', 'chrote-external'))
    expect(await screen.findByText('Hidden blocker')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Flow' })).toHaveAttribute('aria-selected', 'true')
  })

  it('links a loaded closed row back to unfinished work in Flow', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')
    fireEvent.click(screen.getByRole('button', { name: 'chrote' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Closed' }))
    fireEvent.contextMenu(await screen.findByText('Finished dashboard pass'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open in Flow' }))

    expect(await screen.findByText('Title of chrote-ep')).toBeInTheDocument()
    expect(screen.getByText('Finished dashboard pass')).toBeInTheDocument()
  })

  it('reports when a linked row cannot be hydrated for Flow', async () => {
    mockState.beadDetails.set('/srv/chrote\u0000chrote-ep.1', new Error('detail unavailable'))
    render(<BeadsView />)
    fireEvent.contextMenu(await screen.findByText('Title of chrote-ep.1'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open in Flow' }))

    await waitFor(() => expect(mockState.announce).toHaveBeenCalledWith(
      'Flow unavailable · chrote-ep.1: detail unavailable',
      'error',
    ))
    expect(screen.getByRole('tab', { name: 'Map' })).toHaveAttribute('aria-selected', 'true')
  })

  it('keeps the Closed loading state visible until the selected store answers', async () => {
    let answer!: (value: unknown) => void
    mockState.fetchClosedBeadWork.mockImplementation((path: string) => new Promise(resolve => {
      if (path === '/srv/chrote') answer = resolve
      else resolve({ prefix: '', projectPath: path, beads: [] })
    }))
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')
    fireEvent.click(screen.getByRole('button', { name: 'chrote' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Closed' }))

    expect(screen.getByText('Reading closed Beads…')).toBeInTheDocument()
    await act(async () => answer(mockState.closed.get('/srv/chrote')))
    expect(await screen.findByText('Finished dashboard pass')).toBeInTheDocument()
  })

  it('groups formulas, template protos and molecules and exposes their full structure', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')
    fireEvent.click(screen.getByRole('button', { name: 'chrote' }))

    const release = await screen.findByRole('button', { name: 'release' })
    expect(screen.getByText('Template protos')).toBeInTheDocument()
    expect(screen.getByText('Molecules')).toBeInTheDocument()
    expect(screen.getByText('Release template')).toBeInTheDocument()
    expect(screen.getByText('September release')).toBeInTheDocument()

    fireEvent.click(release)
    expect(await screen.findByRole('heading', { name: 'release', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('/srv/chrote/.beads/formulas/release.formula.toml')).toBeInTheDocument()
    expect(screen.getByText('Target')).toBeInTheDocument()
    expect(screen.getByText('Depends on')).toBeInTheDocument()
    expect(document.querySelector('.beads-template-detail button')).toBeNull()

    fireEvent.click(screen.getByText('September release'))
    expect(await screen.findByRole('heading', { name: 'September release', level: 1 })).toBeInTheDocument()
    expect(screen.getByText('Dependencies')).toBeInTheDocument()
    expect(screen.getByText('in progress')).toBeInTheDocument()
    expect(screen.getByText('prod')).toBeInTheDocument()
  })

  it('states when a store has no formulas or molecules', async () => {
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')
    fireEvent.click(screen.getByRole('button', { name: 'ctx' }))
    expect(await screen.findByText('No formulas or molecules in this store.')).toBeInTheDocument()
    expect(screen.getByText('Title of ctx-t4ak')).toBeInTheDocument()
  })

  it('reports template catalog failures without replacing open work', async () => {
    mockState.formulas.set('/srv/chrote', new Error('formula directory unreadable'))
    mockState.molecules.set('/srv/chrote', new Error('molecule query failed'))
    render(<BeadsView />)
    await screen.findByText('Title of ctx-t4ak')
    fireEvent.click(screen.getByRole('button', { name: 'chrote' }))

    expect(await screen.findByText('Formulas: formula directory unreadable')).toBeInTheDocument()
    expect(screen.getByText('Molecules: molecule query failed')).toBeInTheDocument()
    expect(screen.getByText('Title of chrote-ep')).toBeInTheDocument()
  })
})
