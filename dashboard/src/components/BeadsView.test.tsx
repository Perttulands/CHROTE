import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BeadsView from './BeadsView'
import type { BeadsProject, InsightsResponse, TriageResponse } from './BeadsView/types'

const mocks = vi.hoisted(() => ({
  refreshIssues: vi.fn(),
  refreshTriage: vi.fn(),
  refreshInsights: vi.fn(),
  projects: [] as BeadsProject[],
  triage: null as TriageResponse | null,
  insights: null as InsightsResponse | null,
}))

vi.mock('./BeadsView/hooks', () => ({
  useProjects: () => ({
    projects: mocks.projects,
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
  useIssues: () => ({
    issues: [
      {
        id: 'chrt-3dra',
        title: 'Add lightweight right-click menus',
        status: 'open',
        priority: 3,
        type: 'task',
      },
    ],
    loading: false,
    error: null,
    refresh: mocks.refreshIssues,
  }),
  useTriage: () => ({
    triage: mocks.triage,
    loading: false,
    error: null,
    refresh: mocks.refreshTriage,
  }),
  useInsights: () => ({
    insights: mocks.insights,
    loading: false,
    error: null,
    refresh: mocks.refreshInsights,
  }),
  useIssueDetail: () => ({
    issue: {
      id: 'chrt-3dra',
      title: 'Add lightweight right-click menus',
      status: 'open',
      priority: 3,
      type: 'task',
    },
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
  useIssueComments: () => ({
    comments: [],
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
  addIssueComment: vi.fn(),
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.projects = [
    { name: 'chrote', path: '/home/operator/chrote', beadsPath: '/home/operator/chrote/.beads', source: 'configured' },
    { name: 'other', path: '/home/operator/other', beadsPath: '/home/operator/other/.beads', source: 'discovered' },
  ]
  mocks.triage = {
    recommendations: [{ issueId: 'chrt-3dra', rank: 1, reasoning: 'Small and ready', estimatedImpact: 'high' }],
    quickWins: ['chrt-3dra'],
    blockers: ['chrt-3dra'],
  }
  mocks.insights = {
    issueCount: 4,
    openCount: 2,
    blockedCount: 1,
    closedCount: 1,
    health: { score: 75, risks: ['Cycle risk'], warnings: ['One blocker'] },
  }
  Object.assign(navigator, {
    clipboard: {
      writeText: vi.fn().mockResolvedValue(undefined),
    },
  })
})

describe('BeadsView context menus', () => {
  it('renders project, board, triage, and insight values in their existing views', async () => {
    const { container } = render(<BeadsView />)

    const selector = await screen.findByRole('combobox', { name: 'Project' })
    expect(within(selector).getAllByRole('option')).toHaveLength(3)
    expect(selector).toHaveValue('/home/operator/chrote')
    expect(screen.getAllByRole('button', { name: /Kanban|Triage|Insights/ })).toHaveLength(3)
    expect(screen.getByRole('button', { name: 'Kanban' })).toHaveClass('active')
    expect(container.querySelectorAll('.kanban-column')).toHaveLength(6)
    expect(screen.getByText('Add lightweight right-click menus')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Triage' }))
    expect(screen.getByRole('heading', { name: 'Recommended Next' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Quick Wins' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Blockers' })).toBeInTheDocument()
    expect(screen.getByText('Small and ready')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Insights' }))
    expect(screen.getByText('75')).toBeInTheDocument()
    expect(container.querySelectorAll('.stat-card')).toHaveLength(4)
    expect(screen.getByText('Cycle risk')).toBeInTheDocument()
    expect(screen.getByText('One blocker')).toBeInTheDocument()
  })

  it('shows the empty project state when discovery returns no workspaces', () => {
    mocks.projects = []

    render(<BeadsView />)

    expect(screen.getByRole('heading', { name: 'No Beads Projects' })).toBeInTheDocument()
    expect(screen.getByRole('combobox')).toBeDisabled()
  })

  it('hides legacy Patrols UI from the visible Beads status strip', async () => {
    render(<BeadsView />)

    await screen.findByText('Add lightweight right-click menus')

    expect(screen.queryByText('Show patrols')).not.toBeInTheDocument()
  })

  it('offers copy/open/refresh issue-card actions without mutation actions', async () => {
    render(<BeadsView />)
    const issueCard = await screen.findByText('Add lightweight right-click menus')

    fireEvent.contextMenu(issueCard.closest('.issue-card') as HTMLElement)
    const menu = document.querySelector('.beads-context-menu') as HTMLElement

    expect(within(menu).getByRole('button', { name: 'Open Details' })).toBeInTheDocument()
    expect(within(menu).getByRole('button', { name: 'Copy Bead ID' })).toBeInTheDocument()
    expect(within(menu).getByRole('button', { name: 'Copy Bead Reference' })).toBeInTheDocument()
    expect(within(menu).getByRole('button', { name: 'Copy bd show Command' })).toBeInTheDocument()
    expect(within(menu).getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
    expect(within(menu).queryByRole('button', { name: /Close|Claim|Assign|Label|Comment/ })).not.toBeInTheDocument()

    fireEvent.click(within(menu).getByRole('button', { name: 'Copy Bead Reference' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('chrt-3dra — Add lightweight right-click menus')

    fireEvent.contextMenu(issueCard.closest('.issue-card') as HTMLElement)
    const secondMenu = document.querySelector('.beads-context-menu') as HTMLElement
    fireEvent.click(within(secondMenu).getByRole('button', { name: 'Open Details' }))
    expect(await screen.findByRole('dialog', { name: 'Add lightweight right-click menus' })).toBeInTheDocument()
  })

  it('offers project-level copy, refresh, and open-in-files actions', async () => {
    const openProjectInFiles = vi.fn()
    render(<BeadsView onOpenProjectInFiles={openProjectInFiles} />)

    await screen.findByText('Add lightweight right-click menus')
    const statusStrip = document.querySelector('.beads-status-strip') as HTMLElement
    fireEvent.contextMenu(statusStrip)
    const menu = document.querySelector('.beads-context-menu') as HTMLElement

    fireEvent.click(within(menu).getByRole('button', { name: 'Copy Active Project Path' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/home/operator/chrote')

    fireEvent.contextMenu(statusStrip)
    const secondMenu = document.querySelector('.beads-context-menu') as HTMLElement
    fireEvent.click(within(secondMenu).getByRole('button', { name: 'Open Project in Files' }))
    expect(openProjectInFiles).toHaveBeenCalledWith('/home/operator/chrote')

    fireEvent.contextMenu(statusStrip)
    const thirdMenu = document.querySelector('.beads-context-menu') as HTMLElement
    fireEvent.click(within(thirdMenu).getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(mocks.refreshIssues).toHaveBeenCalled())
  })
})
