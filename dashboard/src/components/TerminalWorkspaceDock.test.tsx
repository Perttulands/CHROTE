import { useCallback, useEffect, useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalWorkspaceDock from './TerminalWorkspaceDock'
import {
  readSessionsDockState,
  writeSessionsDockState,
  type SessionsDockState,
} from './workspaceFilesState'

const mocks = vi.hoisted(() => ({
  narrow: false,
  sessions: [{ name: 'shell' }, { name: 'codex' }],
}))

vi.mock('../hooks/useMediaQuery', () => ({
  useMediaQuery: () => mocks.narrow,
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({ sessions: mocks.sessions }),
}))

vi.mock('./SessionPanel', () => ({
  default: (props: {
    collapsed: boolean
    pinned: boolean
    canPin: boolean
    panelId: string
    onTogglePin: () => void
    onClose: () => void
  }) => props.collapsed ? null : (
    <aside id={props.panelId} data-testid="sessions-panel" data-pinned={String(props.pinned)}>
      {props.canPin && <button onClick={props.onTogglePin}>Pin sessions</button>}
      <button onClick={props.onClose}>Close sessions</button>
    </aside>
  ),
}))

vi.mock('./TerminalFilesPanel', () => ({
  default: (props: {
    collapsed: boolean
    pinned: boolean
    canPin: boolean
    panelId: string
    onTogglePin: () => void
    onClose: () => void
  }) => props.collapsed ? null : (
    <aside id={props.panelId} data-testid="files-panel" data-pinned={String(props.pinned)}>
      {props.canPin && <button onClick={props.onTogglePin}>Pin files</button>}
      <button onClick={props.onClose}>Close files</button>
    </aside>
  ),
}))

vi.mock('./TerminalArea', () => ({
  default: ({ sidecarControls }: { sidecarControls?: React.ReactNode }) => (
    <main data-testid="terminal-area">{sidecarControls}</main>
  ),
}))

function DockHarness() {
  const [sessionsDockState, setSessionsDockState] = useState<SessionsDockState>(readSessionsDockState)
  const [filesOpen, setFilesOpen] = useState(false)
  const handleFilesOpenChange = useCallback((_workspaceId: string, open: boolean) => {
    setFilesOpen(open)
  }, [])

  useEffect(() => {
    writeSessionsDockState(sessionsDockState)
  }, [sessionsDockState])

  return (
    <TerminalWorkspaceDock
      workspaceId="terminal1"
      active
      sessionsDockState={sessionsDockState}
      onSessionsDockStateChange={setSessionsDockState}
      sessionsForcedPinned={filesOpen}
      onFilesOpenChange={handleFilesOpenChange}
      onOpenSessionBankSettings={vi.fn()}
      onOpenInFiles={vi.fn()}
    />
  )
}

function renderDock() {
  return render(<DockHarness />)
}

describe('TerminalWorkspaceDock sidecar state machine', () => {
  beforeEach(() => {
    localStorage.clear()
    mocks.narrow = false
  })

  it('persists a fresh closed default before later legacy dashboard writes can reopen Sessions', () => {
    const firstMount = renderDock()
    expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()
    firstMount.unmount()

    localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: false }))
    renderDock()

    expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem('chrote.sessionsDock.v1') || '{}')).toEqual({
      version: 1,
      state: { open: false, pinned: false, width: 260, searchTerm: '', collapsedGroups: [] },
    })
  })

  it('opens and toggles each sidecar independently from a stable switcher', () => {
    const { container } = renderDock()
    const sessions = screen.getByRole('button', { name: /Sessions sidecar/i })
    const files = screen.getByRole('button', { name: /Files sidecar/i })

    expect(sessions).toHaveAttribute('aria-pressed', 'false')
    expect(files).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()
    expect(screen.queryByTestId('files-panel')).not.toBeInTheDocument()

    fireEvent.click(sessions)
    expect(sessions).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-pinned', 'false')
    expect(screen.queryByTestId('files-panel')).not.toBeInTheDocument()
    expect(container.querySelector('.terminal-sidecar-dismiss')).toBeInTheDocument()

    fireEvent.click(files)
    expect(files).toHaveAttribute('aria-pressed', 'true')
    expect(sessions).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('sessions-panel')).toBeInTheDocument()
    expect(screen.getByTestId('files-panel')).toBeInTheDocument()
    expect(container.querySelector('.terminal-sidecar-dismiss')).not.toBeInTheDocument()

    fireEvent.click(files)
    expect(files).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByTestId('files-panel')).not.toBeInTheDocument()
    expect(screen.getByTestId('sessions-panel')).toBeInTheDocument()

    fireEvent.click(sessions)
    expect(sessions).toHaveAttribute('aria-pressed', 'false')
    expect(container.querySelector('.terminal-sidecar-dismiss')).not.toBeInTheDocument()
  })

  it('keeps Sessions and Files open together and closes them independently', () => {
    renderDock()
    const sessions = screen.getByRole('button', { name: /Sessions sidecar/i })
    const files = screen.getByRole('button', { name: /Files sidecar/i })

    fireEvent.click(sessions)
    fireEvent.click(files)

    expect(sessions).toHaveAttribute('aria-pressed', 'true')
    expect(files).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByTestId('sessions-panel')).toBeInTheDocument()
    expect(screen.getByTestId('files-panel')).toBeInTheDocument()
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-pinned', 'true')
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-pinned', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Close sessions' }))

    expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()
    expect(screen.getByTestId('files-panel')).toBeInTheDocument()
    expect(files).toHaveAttribute('aria-pressed', 'true')
  })

  it('pins only on desktop and closes an unpinned sidecar through Escape or the dismiss layer', () => {
    const { container } = renderDock()
    fireEvent.click(screen.getByRole('button', { name: /Sessions sidecar/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Pin sessions' }))

    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-pinned', 'true')
    expect(container.querySelector('.terminal-sidecar-dismiss')).not.toBeInTheDocument()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.getByTestId('sessions-panel')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Pin sessions' }))
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Files sidecar/i }))
    fireEvent.click(container.querySelector('.terminal-sidecar-dismiss')!)
    expect(screen.queryByTestId('files-panel')).not.toBeInTheDocument()
  })

  it('ignores hidden keep-alive dialogs for Escape while deferring to a visible dialog', () => {
    const { container } = renderDock()
    const hiddenHost = document.createElement('div')
    const dialog = document.createElement('section')
    hiddenHost.style.display = 'none'
    dialog.setAttribute('role', 'dialog')
    Object.defineProperty(dialog, 'getClientRects', {
      configurable: true,
      value: () => [{ width: 100, height: 100 }],
    })
    hiddenHost.append(dialog)
    document.body.append(hiddenHost)

    try {
      fireEvent.click(screen.getByRole('button', { name: /Sessions sidecar/i }))
      fireEvent.keyDown(window, { key: 'Escape' })
      expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: /Sessions sidecar/i }))
      hiddenHost.style.display = 'block'
      fireEvent.keyDown(window, { key: 'Escape' })
      expect(screen.getByTestId('sessions-panel')).toBeInTheDocument()

      hiddenHost.remove()
      fireEvent.keyDown(window, { key: 'Escape' })
      expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()
      expect(container.querySelector('.terminal-sidecar-dismiss')).not.toBeInTheDocument()
    } finally {
      hiddenHost.remove()
    }
  })

  it('reopens a closed Sessions panel pinned without leaking that preference into Files', () => {
    renderDock()
    const sessions = screen.getByRole('button', { name: /Sessions sidecar/i })
    fireEvent.click(sessions)
    fireEvent.click(screen.getByRole('button', { name: 'Pin sessions' }))
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-pinned', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Close sessions' }))
    expect(screen.queryByTestId('sessions-panel')).not.toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem('chrote.sessionsDock.v1') || '{}')).toMatchObject({
      state: { open: false, pinned: true },
    })

    fireEvent.click(sessions)
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-pinned', 'true')

    fireEvent.click(sessions)
    fireEvent.click(screen.getByRole('button', { name: /Files sidecar/i }))
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-pinned', 'false')
  })

  it('forces a stored desktop pin into overlay presentation on narrow viewports without losing the stored preference', () => {
    writeSessionsDockState({
      open: true,
      pinned: true,
      width: 300,
      searchTerm: '',
      collapsedGroups: [],
    })
    mocks.narrow = true

    const { container } = renderDock()
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-pinned', 'false')
    expect(screen.queryByRole('button', { name: 'Pin sessions' })).not.toBeInTheDocument()
    expect(container.querySelector('.terminal-sidecar-dismiss')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sessions sidecar' }).querySelector('.terminal-sidecar-label')).toBeInTheDocument()

    expect(JSON.parse(localStorage.getItem('chrote.sessionsDock.v1') || '{}')).toMatchObject({
      state: { open: true, pinned: true },
    })
  })
})
