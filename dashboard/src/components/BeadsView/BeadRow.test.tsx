import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import BeadRow from './BeadRow'
import { resetBeadCardForTest, useBeadCardRequest } from '../../beads/beadCard'
import type { WorkRow } from '../../beads/beadsTree'

const mockState = vi.hoisted(() => ({
  openSendToSession: vi.fn(),
  announce: vi.fn(),
  copy: vi.fn(),
}))

vi.mock('../../context/SessionContext', () => ({
  useSession: () => ({ openSendToSession: mockState.openSendToSession }),
}))

vi.mock('../../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

vi.mock('../../utils/clipboard', () => ({
  copyAndAnnounce: (text: string, what: string, announce: unknown) => mockState.copy(text, what, announce),
}))

function row(id: string, title: string): WorkRow {
  return { id, title, status: 'open', type: 'task', priority: 1, blocked: false, projectPath: '/srv/chrote', projectName: 'chrote' }
}

function fold(expanded: boolean, count = 1) {
  return { count, expanded, setExpanded: vi.fn(), setSubtreeExpanded: vi.fn() }
}

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request?.id ?? 'none'}</span>
}

const menuItems = () => screen.getAllByRole('menuitem').map(item => item.querySelector('.menu-row-label')?.textContent)

afterEach(() => {
  resetBeadCardForTest()
  mockState.openSendToSession.mockReset()
  mockState.announce.mockReset()
  mockState.copy.mockReset()
})

describe('a Bead row', () => {
  it('opens the card and folds its children from one click anywhere on it', () => {
    const epic = fold(true, 3)
    render(
      <>
        <BeadRow row={row('chrote-ep', 'The epic')} fold={epic} />
        <CardProbe />
      </>,
    )

    expect(screen.getByRole('button', { name: /The epic/ })).toHaveTextContent('▾3')
    fireEvent.click(screen.getByText('The epic'))

    expect(screen.getByTestId('card-request')).toHaveTextContent('chrote-ep')
    expect(epic.setExpanded).toHaveBeenCalledWith(false)
  })

  it('opens and closes children with the arrows, and moves between rows with them', () => {
    const epic = fold(false)
    render(
      <div className="beads-content">
        <BeadRow row={row('chrote-ep', 'The epic')} fold={epic} />
        <BeadRow row={row('chrote-ep.1', 'A child')} depth={1} />
      </div>,
    )
    const epicRow = screen.getByRole('button', { name: /The epic/ })
    const child = screen.getByRole('button', { name: /A child/ })
    expect(epicRow).toHaveTextContent('▸1')

    fireEvent.keyDown(epicRow, { key: 'ArrowRight' })
    expect(epic.setExpanded).toHaveBeenLastCalledWith(true)
    fireEvent.keyDown(epicRow, { key: 'ArrowLeft' })
    expect(epic.setExpanded).toHaveBeenLastCalledWith(false)

    epicRow.focus()
    fireEvent.keyDown(epicRow, { key: 'ArrowDown' })
    expect(child).toHaveFocus()
    fireEvent.keyDown(child, { key: 'ArrowUp' })
    expect(epicRow).toHaveFocus()

    // A row without children has nothing for the sideways arrows to do.
    fireEvent.keyDown(child, { key: 'ArrowRight' })
    expect(epic.setExpanded).toHaveBeenCalledTimes(2)
  })

  it('says its type as a word, so the colour is never the only signal', () => {
    render(
      <div className="beads-content">
        {['task', 'decision', 'bug', 'epic', 'feature', 'chore', ''].map((type, index) => (
          <BeadRow key={type || 'none'} row={{ ...row(`chrote-t${index}`, `A ${type || 'typeless'} Bead`), type }} />
        ))}
        <BeadRow row={{ ...row('chrote-done', 'A finished Bead'), status: 'closed' }} />
      </div>,
    )

    const words = screen.getAllByRole('button', { name: /Bead/ }).map(button => button.querySelector('.bead-type')?.textContent)
    // The seventh row records no type; bd calls that a task and so does the row.
    expect(words).toEqual(['TASK', 'DECISION', 'BUG', 'EPIC', 'FEATURE', 'CHORE', 'TASK', 'TASK'])
    expect(screen.getByRole('button', { name: /A finished Bead/ })).toHaveTextContent('✓')
  })

  it('offers its actions from a right-click, with the whole subtree on a row with children', () => {
    const epic = fold(true, 2)
    render(
      <>
        <BeadRow row={row('chrote-ep', 'The epic')} fold={epic} />
        <BeadRow row={row('chrote-ep.1', 'A child')} depth={1} />
        <CardProbe />
      </>,
    )

    fireEvent.contextMenu(screen.getByText('The epic'))
    expect(menuItems()).toEqual(['Open', 'Send', 'Copy id', 'Copy id and title', 'Expand all', 'Collapse all'])
    expect(screen.getByRole('menuitem', { name: 'Send' })).toHaveAttribute('aria-keyshortcuts', 'Alt+S')

    fireEvent.click(screen.getByRole('menuitem', { name: 'Collapse all' }))
    expect(epic.setSubtreeExpanded).toHaveBeenCalledWith(false)
    expect(screen.queryByRole('menu')).toBeNull()

    fireEvent.contextMenu(screen.getByText('A child'))
    expect(menuItems()).toEqual(['Open', 'Send', 'Copy id', 'Copy id and title'])

    fireEvent.click(screen.getByRole('menuitem', { name: 'Send' }))
    expect(mockState.openSendToSession).toHaveBeenCalledWith({ reference: 'bead chrote-ep.1: A child' })

    fireEvent.contextMenu(screen.getByText('A child'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Copy id and title' }))
    expect(mockState.copy).toHaveBeenCalledWith('chrote-ep.1: A child', 'chrote-ep.1 and its title', mockState.announce)

    fireEvent.contextMenu(screen.getByText('A child'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open' }))
    expect(screen.getByTestId('card-request')).toHaveTextContent('chrote-ep.1')
  })
})
