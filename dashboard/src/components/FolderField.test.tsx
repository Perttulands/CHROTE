import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FolderField from './FolderField'
import { fetchDirectory } from './FilesView/fileService'

vi.mock('../workspaces/workspacesApi', async () => {
  const actual = await vi.importActual<typeof import('../workspaces/workspacesApi')>('../workspaces/workspacesApi')
  return {
    ...actual,
    fetchWorkspaces: () => Promise.resolve([
      { path: '/srv/chrote', sources: ['git'], sessions: [], instructions: 2 },
      { path: '/home/operator/repos/VSK-Zone', sources: ['git'], sessions: [], instructions: 0 },
      { path: '/srv/context-citadel', sources: ['git'], sessions: [], instructions: 1 },
    ]),
  }
})

vi.mock('./FilesView/fileService', async () => {
  const actual = await vi.importActual<typeof import('./FilesView/fileService')>('./FilesView/fileService')
  return { ...actual, fetchDirectory: vi.fn() }
})

const mockedFetchDirectory = vi.mocked(fetchDirectory)

const entry = (path: string, isDir: boolean) => ({
  path,
  name: path.split('/').pop() || '/',
  isDir,
  size: 0,
  modified: '2026-01-01T00:00:00Z',
  type: '',
})

const onChange = vi.fn()
const onSubmit = vi.fn()

function Harness({ initial = '', recents = [] as string[] }) {
  const [value, setValue] = useState(initial)
  return (
    <FolderField
      value={value}
      onChange={next => { setValue(next); onChange(next) }}
      onSubmit={onSubmit}
      recents={recents}
      ariaLabel="Folder"
    />
  )
}

const options = () => screen.getAllByRole('option').map(option => option.textContent)
const highlighted = () => screen.queryByRole('option', { selected: true })?.textContent ?? null

describe('FolderField', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockedFetchDirectory.mockImplementation(async path => {
      if (path === '/home/operator/') {
        return [entry('/home/operator/.cache', true), entry('/home/operator/notes.txt', false), entry('/home/operator/research', true), entry('/home/operator/repos', true)]
      }
      if (path === '/home/operator/repos/') return [entry('/home/operator/repos/VSK-Zone', true), entry('/home/operator/repos/vsk-notes', true)]
      return []
    })
  })

  it('offers the recent folders until something is typed, and takes a click', async () => {
    render(<Harness initial="/srv/chrote" recents={['/srv/chrote', '/srv']} />)

    expect(options()).toEqual(['/srv/chrote', '/srv'])
    expect(highlighted()).toBeNull()

    fireEvent.click(screen.getByRole('option', { name: '/srv' }))
    expect(onChange).toHaveBeenLastCalledWith('/srv')
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('ranks the workspaces by a typed fragment with the best first, and Enter takes it', async () => {
    render(<Harness />)
    const field = screen.getByLabelText('Folder')

    fireEvent.change(field, { target: { value: 'rep VSK' } })

    await waitFor(() => expect(options()).toEqual(['/home/operator/repos/VSK-Zone']))
    expect(highlighted()).toBe('/home/operator/repos/VSK-Zone')

    fireEvent.keyDown(field, { key: 'Enter' })
    expect(onChange).toHaveBeenLastCalledWith('/home/operator/repos/VSK-Zone')
    expect(onSubmit).toHaveBeenCalledWith('/home/operator/repos/VSK-Zone')
  })

  it('completes the directories under a typed path with Tab, dot-folders last', async () => {
    render(<Harness />)
    const field = screen.getByLabelText('Folder')

    fireEvent.change(field, { target: { value: '/home/operator/' } })
    await waitFor(() => expect(options()).toEqual(['/home/operator/research', '/home/operator/repos', '/home/operator/.cache']))
    expect(highlighted()).toBeNull()

    fireEvent.change(field, { target: { value: '/home/operator/re' } })
    await waitFor(() => expect(options()).toEqual(['/home/operator/research', '/home/operator/repos']))

    fireEvent.keyDown(field, { key: 'ArrowDown' })
    fireEvent.keyDown(field, { key: 'ArrowDown' })
    expect(highlighted()).toBe('/home/operator/repos')

    fireEvent.keyDown(field, { key: 'Tab' })
    expect(onChange).toHaveBeenLastCalledWith('/home/operator/repos/')
    await waitFor(() => expect(options()).toEqual(['/home/operator/repos/VSK-Zone', '/home/operator/repos/vsk-notes']))

    // Nothing highlighted: Enter means the typed folder itself.
    fireEvent.keyDown(field, { key: 'Enter' })
    expect(onSubmit).toHaveBeenCalledWith('/home/operator/repos/')
  })

  it('says so when a typed folder cannot be listed, and still takes the path', async () => {
    mockedFetchDirectory.mockRejectedValue(new Error('Permission denied'))
    render(<Harness />)
    const field = screen.getByLabelText('Folder')

    fireEvent.change(field, { target: { value: '/root/private' } })

    await waitFor(() => expect(screen.getByText('Cannot list this folder')).toBeInTheDocument())
    fireEvent.keyDown(field, { key: 'Enter' })
    expect(onSubmit).toHaveBeenCalledWith('/root/private')
  })
})
