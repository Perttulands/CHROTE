import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { BeadProject } from '../beads/beadsApi'
import { resetSurfacesForTest } from '../keys/dismiss'
import ReportBox, { splitReport } from './ReportBox'

const PROJECTS: BeadProject[] = [
  { name: 'ctx', path: '/code/ctx', beadsPath: '/code/ctx/.beads', prefix: 'ctx' },
  { name: 'chrote', path: '/code/chrote', beadsPath: '/code/chrote/.beads', prefix: 'chrote' },
  { name: 'media', path: '/code/media', beadsPath: '/code/media/.beads', prefix: 'media' },
]

const mocks = vi.hoisted(() => ({
  createBead: vi.fn(),
  openSendToSession: vi.fn(),
  announce: vi.fn(),
}))

vi.mock('../beads/beadsApi', () => ({ createBead: mocks.createBead }))
vi.mock('../beads/beadIds', () => ({
  beadProjects: () => PROJECTS,
  ensureBeadProjects: () => Promise.resolve(PROJECTS),
}))
vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ openSendToSession: mocks.openSendToSession }),
}))
vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mocks.announce }),
}))
vi.mock('../context/useFocusedSession', () => ({
  useFocusedSession: () => 'alice:shell',
}))
vi.mock('./TerminalPool', () => ({
  useTerminalPool: () => ({ terminals: new Map(), connectionStates: new Map() }),
}))

let setOpen: (open: boolean) => void = () => {}

function Harness() {
  const [open, setOpenState] = useState(true)
  setOpen = setOpenState
  return <ReportBox open={open} onClose={() => setOpenState(false)} activeTab="terminal2" />
}

function field() {
  return screen.getByRole('textbox', { name: 'Issue' })
}

describe('ReportBox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetSurfacesForTest()
  })

  it('reads the first line as the title and the rest as the description', () => {
    expect(splitReport('  Title \n\nmore\nlines\n')).toEqual({ title: 'Title', description: 'more\nlines' })
    expect(splitReport('only')).toEqual({ title: 'only', description: '' })
  })

  it('files a feature into a picked store without context, then opens reset', async () => {
    mocks.createBead.mockResolvedValue({ id: 'media-x1', title: 'Wish' })
    render(<Harness />)

    fireEvent.change(field(), { target: { value: 'Wish' } })
    const type = screen.getByRole('radio', { name: 'Bug' })
    fireEvent.keyDown(type, { key: 'ArrowRight' })
    expect(screen.getByRole('radio', { name: 'Feature' })).toHaveAttribute('aria-checked', 'true')
    fireEvent.click(screen.getByRole('checkbox'))

    const store = screen.getByRole('combobox', { name: 'Store' })
    fireEvent.focus(store)
    fireEvent.change(store, { target: { value: 'med' } })
    expect(screen.getAllByRole('option')).toHaveLength(1)
    fireEvent.keyDown(store, { key: 'Enter' })
    expect(mocks.createBead).not.toHaveBeenCalled()
    fireEvent.blur(store)
    expect(store).toHaveValue('media')

    fireEvent.keyDown(field(), { key: 'Enter' })
    await waitFor(() => expect(mocks.createBead).toHaveBeenCalledWith({
      path: '/code/media', title: 'Wish', description: '', type: 'feature',
    }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    act(() => setOpen(true))
    expect(field()).toHaveValue('')
    expect(screen.getByRole('combobox', { name: 'Store' })).toHaveValue('chrote')
    expect(screen.getByRole('radio', { name: 'Bug' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('checkbox')).toBeChecked()
  })

  it('keeps an unfiled draft across Escape', () => {
    render(<Harness />)
    fireEvent.change(field(), { target: { value: 'half a thought' } })
    fireEvent.keyDown(field(), { key: 'Escape' })
    expect(screen.queryByRole('dialog')).toBeNull()

    act(() => setOpen(true))
    expect(field()).toHaveValue('half a thought')
    expect(field()).toHaveFocus()
  })

  it('keeps the box open with the text and the server words when filing fails', async () => {
    mocks.createBead.mockRejectedValue(new Error('bd: database is locked'))
    render(<Harness />)

    fireEvent.change(field(), { target: { value: 'Broken' } })
    fireEvent.keyDown(field(), { key: 'Enter', ctrlKey: true })

    expect(await screen.findByRole('alert')).toHaveTextContent('bd: database is locked')
    expect(field()).toHaveValue('Broken')
    expect(mocks.openSendToSession).not.toHaveBeenCalled()
    expect(mocks.announce).not.toHaveBeenCalled()
  })
})
