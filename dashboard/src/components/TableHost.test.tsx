import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TableHost, TableSlot } from './TableHost'
import { TableProvider, clearTable, putOnTable, readTable, resetTableForTest } from '../context/TableContext'
import { openBeadCard } from '../beads/beadCard'
import { resetBeadProjectsForTest, setBeadProjects } from '../beads/beadIds'
import { resetKnownBeadsForTest } from '../beads/knownBeads'
import { openAgentContext } from '../agents/agentContextPanel'
import { resetChordsForTest } from '../keys/chords'
import { DEFAULT_SETTINGS } from '../types'

const api = vi.hoisted(() => ({
  fetchBead: vi.fn(),
  readTextFile: vi.fn(),
  fetchFileDiff: vi.fn(),
  fetchAgentContext: vi.fn(),
  fetchAgentFile: vi.fn(),
  send: vi.fn(),
  updateSettings: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: DEFAULT_SETTINGS, updateSettings: api.updateSettings,
    sessions: [], openSendToSession: api.send,
  }),
}))
vi.mock('../context/StatusContext', () => ({ useStatus: () => ({ announce: vi.fn() }) }))
vi.mock('../beads/beadsApi', () => ({
  fetchBeadProjects: () => Promise.resolve([]),
  fetchBead: (...args: unknown[]) => api.fetchBead(...args),
}))
vi.mock('./FilesView/fileService', async () => ({
  ...await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService'),
  readTextFile: (...args: unknown[]) => api.readTextFile(...args),
  fetchFileDiff: (...args: unknown[]) => api.fetchFileDiff(...args),
}))
vi.mock('../agents/agentContextApi', async () => ({
  ...await vi.importActual<typeof import('../agents/agentContextApi')>('../agents/agentContextApi'),
  fetchAgentContext: (...args: unknown[]) => api.fetchAgentContext(...args),
  fetchAgentFile: (...args: unknown[]) => api.fetchAgentFile(...args),
}))

function Dashboard({ tab, oldSlot = true }: { tab: 'first' | 'second' | 'unsupported'; oldSlot?: boolean }) {
  return (
    <TableProvider>
      <TableHost>
        <div>Terminal workspace</div>
        {oldSlot && <section aria-label="first" hidden={tab !== 'first'}><TableSlot active={tab === 'first'} /></section>}
        <section aria-label="second" hidden={tab !== 'second'}><TableSlot active={tab === 'second'} /></section>
      </TableHost>
    </TableProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  api.fetchBead.mockImplementation((_path: string, id: string) => Promise.resolve({
    id, title: `Title ${id}`, status: 'open', type: 'task', priority: 1,
    updated: '', parents: [], children: [], blockedBy: [], blocks: [],
  }))
  api.readTextFile.mockResolvedValue('Original file')
  api.fetchFileDiff.mockResolvedValue({ repository: false, diff: '' })
  api.fetchAgentContext.mockResolvedValue({
    folder: '/project', harness: 'claude-code', user: 'operator',
    instructions: [{ path: '/project/CLAUDE.md', scope: 'project', kind: 'CLAUDE.md', readable: true, size: 20 }],
    skills: [], memories: [],
  })
  api.fetchAgentFile.mockResolvedValue('Original instruction')
  setBeadProjects([{ name: 'test', path: '/project', beadsPath: '/project/.beads', prefix: 'test' }])
})

afterEach(() => {
  resetTableForTest()
  resetBeadProjectsForTest()
  resetKnownBeadsForTest()
  resetChordsForTest()
})

describe('the persistent table', () => {
  it('reads one Bead per selection, carries its trail across slots, and refreshes a new request', async () => {
    const view = render(<Dashboard tab="first" />)
    act(() => openBeadCard('test-one', '/project'))
    await screen.findByText('Title test-one')
    const table = screen.getByRole('complementary')
    expect(api.fetchBead).toHaveBeenCalledTimes(1)

    view.rerender(<Dashboard tab="second" />)
    expect(screen.getByRole('region', { name: 'second' })).toContainElement(table)
    expect(api.fetchBead).toHaveBeenCalledTimes(1)

    act(() => putOnTable({ kind: 'bead', id: 'test-two', projectPath: '/project', trail: ['test-one'] }))
    await screen.findByText('Title test-two')
    view.rerender(<Dashboard tab="unsupported" />)
    expect(screen.queryByRole('complementary')).toBeNull()
    fireEvent.keyDown(document, { key: 'Escape' })
    fireEvent.keyDown(document, { key: 's', altKey: true })
    expect(readTable()).toMatchObject({ id: 'test-two', trail: ['test-one'] })
    expect(api.send).not.toHaveBeenCalled()

    view.rerender(<Dashboard tab="first" />)
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    await screen.findByText('Title test-one')
    expect(api.fetchBead).toHaveBeenCalledTimes(3)
    act(() => openBeadCard('test-one', '/project'))
    await waitFor(() => expect(api.fetchBead).toHaveBeenCalledTimes(4))
  })

  it('keeps a file editor, draft and scroll through parking and slot retirement', async () => {
    const view = render(<Dashboard tab="first" />)
    act(() => putOnTable({ kind: 'file', path: '/project/note.txt' }))
    await screen.findByText('Original file')
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const editor = screen.getByRole('textbox', { name: 'Edit note.txt' })
    fireEvent.change(editor, { target: { value: 'Unsaved change' } })
    editor.scrollTop = 120

    view.rerender(<Dashboard tab="unsupported" />)
    expect(screen.queryByRole('textbox')).toBeNull()
    view.rerender(<Dashboard tab="second" oldSlot={false} />)
    expect(screen.getByRole('textbox', { name: 'Edit note.txt' })).toBe(editor)
    expect(editor).toHaveValue('Unsaved change')
    expect(editor.scrollTop).toBe(120)
    expect(api.readTextFile).toHaveBeenCalledTimes(1)
    expect(api.fetchFileDiff).toHaveBeenCalledTimes(1)

    // The existing file reader does not replace a dirty draft for the same path.
    act(() => putOnTable({ kind: 'file', path: '/project/note.txt' }))
    expect(editor).toHaveValue('Unsaved change')
    expect(api.readTextFile).toHaveBeenCalledTimes(1)
  })

  it('keeps a loaded failure until the object is deliberately reopened', async () => {
    api.readTextFile.mockRejectedValueOnce(new Error('Cannot read this file'))
    const view = render(<Dashboard tab="first" />)
    act(() => putOnTable({ kind: 'file', path: '/project/note.txt' }))
    await screen.findByText('Cannot read this file')
    view.rerender(<Dashboard tab="second" />)
    expect(screen.getByText('Cannot read this file')).toBeVisible()
    expect(api.readTextFile).toHaveBeenCalledTimes(1)

    act(clearTable)
    act(() => putOnTable({ kind: 'file', path: '/project/note.txt' }))
    await screen.findByText('Original file')
    expect(api.readTextFile).toHaveBeenCalledTimes(2)
  })

  it('keeps the instruction reader and its unsaved draft when its tab changes', async () => {
    const view = render(<Dashboard tab="first" />)
    act(() => openAgentContext({
      sessionKey: 'operator:worker', folder: '/project', harness: 'claude-code', user: 'operator', shell: false,
    }))
    fireEvent.click(await screen.findByText('/project/CLAUDE.md'))
    await screen.findByText('Original instruction')
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const editor = screen.getByRole('textbox', { name: 'Edit /project/CLAUDE.md' })
    fireEvent.change(editor, { target: { value: 'Unsaved instruction' } })

    view.rerender(<Dashboard tab="unsupported" />)
    view.rerender(<Dashboard tab="second" />)
    expect(screen.getByRole('textbox', { name: 'Edit /project/CLAUDE.md' })).toBe(editor)
    expect(editor).toHaveValue('Unsaved instruction')
    expect(api.fetchAgentContext).toHaveBeenCalledTimes(1)
    expect(api.fetchAgentFile).toHaveBeenCalledTimes(1)
  })

  it('limits a resize using the active slot layout, beyond the display-contents hosts', async () => {
    const view = render(<Dashboard tab="first" />)
    act(() => openBeadCard('test-one', '/project'))
    await screen.findByText('Title test-one')
    view.rerender(<Dashboard tab="second" />)
    Object.defineProperty(screen.getByRole('region', { name: 'second' }), 'clientWidth', { value: 880 })
    fireEvent.keyDown(screen.getByRole('separator', { name: 'Resize the table' }), { key: 'ArrowLeft' })
    expect(api.updateSettings).toHaveBeenLastCalledWith({ tableWidth: 400 })
  })

  it('contains a reader crash without removing the workspace and lets the operator retry', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const expectedError = (event: ErrorEvent) => { event.preventDefault() }
    window.addEventListener('error', expectedError)
    api.fetchAgentContext.mockResolvedValueOnce({ instructions: null, skills: [], memories: [] })
    try {
      render(<Dashboard tab="first" />)
      act(() => openAgentContext({
        sessionKey: 'operator:worker', folder: '/project', harness: 'claude-code', user: 'operator', shell: false,
      }))
      await screen.findByText('Something went wrong')
      expect(screen.getByText('Terminal workspace')).toBeVisible()
      fireEvent.click(screen.getByRole('button', { name: 'Try Again' }))
      await screen.findByText('/project/CLAUDE.md')
      expect(screen.getByText('Terminal workspace')).toBeVisible()
    } finally {
      window.removeEventListener('error', expectedError)
      consoleError.mockRestore()
    }
  })
})
