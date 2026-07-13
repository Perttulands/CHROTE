import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalWorkspaceDock from './TerminalWorkspaceDock'

vi.mock('./SessionPanel', () => ({
  default: ({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) => (
    <aside data-testid="sessions-panel" data-collapsed={String(collapsed)}>
      <button type="button" onClick={onToggle}>{collapsed ? 'Expand Sessions panel' : 'Collapse Sessions panel'}</button>
    </aside>
  ),
}))

vi.mock('./TerminalFilesPanel', () => ({
  default: ({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) => (
    <aside data-testid="files-panel" data-collapsed={String(collapsed)}>
      <button type="button" onClick={onToggle}>{collapsed ? 'Expand Files panel' : 'Collapse Files panel'}</button>
    </aside>
  ),
}))

vi.mock('./TerminalArea', () => ({
  default: ({ workspaceId }: { workspaceId: string }) => <main data-testid="terminal-area">{workspaceId}</main>,
}))

describe('TerminalWorkspaceDock', () => {
  beforeEach(() => window.localStorage.clear())

  it('supports Sessions, Files, both, and neither without coupling the toggles', () => {
    render(
      <TerminalWorkspaceDock
        workspaceId="terminal1"
        active
        onOpenSessionBankSettings={vi.fn()}
        onOpenInFiles={vi.fn()}
      />,
    )

    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-collapsed', 'false')
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Expand Files panel' }))
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-collapsed', 'false')
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'false')

    fireEvent.click(screen.getByRole('button', { name: 'Collapse Sessions panel' }))
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-collapsed', 'true')
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'false')

    fireEvent.click(screen.getByRole('button', { name: 'Collapse Files panel' }))
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-collapsed', 'true')
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Expand Sessions panel' }))
    expect(screen.getByTestId('sessions-panel')).toHaveAttribute('data-collapsed', 'false')
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'true')
  })

  it('restores panel layout independently for each terminal workspace', () => {
    const first = render(
      <TerminalWorkspaceDock
        workspaceId="terminal1"
        active
        onOpenSessionBankSettings={vi.fn()}
        onOpenInFiles={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Expand Files panel' }))
    first.unmount()

    const second = render(
      <TerminalWorkspaceDock
        workspaceId="terminal2"
        active
        onOpenSessionBankSettings={vi.fn()}
        onOpenInFiles={vi.fn()}
      />,
    )
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'true')
    second.unmount()

    render(
      <TerminalWorkspaceDock
        workspaceId="terminal1"
        active
        onOpenSessionBankSettings={vi.fn()}
        onOpenInFiles={vi.fn()}
      />,
    )
    expect(screen.getByTestId('files-panel')).toHaveAttribute('data-collapsed', 'false')
  })
})
