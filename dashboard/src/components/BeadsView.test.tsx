import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BeadsView from './BeadsView'

const mocks = vi.hoisted(() => ({
  refreshIssues: vi.fn(),
  refreshTriage: vi.fn(),
  refreshInsights: vi.fn(),
}))

vi.mock('./BeadsView/hooks', () => ({
  useProjects: () => ({
    projects: [
      { name: 'chrote', path: '/home/perttu/chrote', beadsPath: '/home/perttu/chrote/.beads', source: 'configured' },
    ],
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
    triage: { recommendations: [], quickWins: [], blockers: [] },
    loading: false,
    error: null,
    refresh: mocks.refreshTriage,
  }),
  useInsights: () => ({
    insights: null,
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
  Object.assign(navigator, {
    clipboard: {
      writeText: vi.fn().mockResolvedValue(undefined),
    },
  })
})

describe('BeadsView context menus', () => {
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
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('/home/perttu/chrote')

    fireEvent.contextMenu(statusStrip)
    const secondMenu = document.querySelector('.beads-context-menu') as HTMLElement
    fireEvent.click(within(secondMenu).getByRole('button', { name: 'Open Project in Files' }))
    expect(openProjectInFiles).toHaveBeenCalledWith('/home/perttu/chrote')

    fireEvent.contextMenu(statusStrip)
    const thirdMenu = document.querySelector('.beads-context-menu') as HTMLElement
    fireEvent.click(within(thirdMenu).getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(mocks.refreshIssues).toHaveBeenCalled())
  })
})
