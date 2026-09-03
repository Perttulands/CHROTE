import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import BeadRow from './BeadRow'
import { resetBeadCardForTest, useBeadCardRequest } from '../../beads/beadCard'
import type { WorkRow } from '../../beads/beadsTree'

function row(id: string, title: string): WorkRow {
  return { id, title, status: 'open', type: 'task', priority: 1, blocked: false, projectPath: '/srv/chrote', projectName: 'chrote' }
}

function CardProbe() {
  const request = useBeadCardRequest()
  return <span data-testid="card-request">{request?.id ?? 'none'}</span>
}

afterEach(() => resetBeadCardForTest())

describe('a Bead row', () => {
  it('opens the card and folds its children from one click anywhere on it', () => {
    const setExpanded = vi.fn()
    render(
      <>
        <BeadRow row={row('chrote-ep', 'The epic')} fold={{ count: 3, expanded: true, setExpanded }} />
        <CardProbe />
      </>,
    )

    expect(screen.getByRole('button', { name: /The epic/ })).toHaveTextContent('▾3')
    fireEvent.click(screen.getByText('The epic'))

    expect(screen.getByTestId('card-request')).toHaveTextContent('chrote-ep')
    expect(setExpanded).toHaveBeenCalledWith(false)
  })

  it('opens and closes children with the arrows, and moves between rows with them', () => {
    const setExpanded = vi.fn()
    render(
      <div className="beads-content">
        <BeadRow row={row('chrote-ep', 'The epic')} fold={{ count: 1, expanded: false, setExpanded }} />
        <BeadRow row={row('chrote-ep.1', 'A child')} depth={1} />
      </div>,
    )
    const epic = screen.getByRole('button', { name: /The epic/ })
    const child = screen.getByRole('button', { name: /A child/ })
    expect(epic).toHaveTextContent('▸1')

    fireEvent.keyDown(epic, { key: 'ArrowRight' })
    expect(setExpanded).toHaveBeenLastCalledWith(true)
    fireEvent.keyDown(epic, { key: 'ArrowLeft' })
    expect(setExpanded).toHaveBeenLastCalledWith(false)

    epic.focus()
    fireEvent.keyDown(epic, { key: 'ArrowDown' })
    expect(child).toHaveFocus()
    fireEvent.keyDown(child, { key: 'ArrowUp' })
    expect(epic).toHaveFocus()

    // A row without children has nothing for the sideways arrows to do.
    fireEvent.keyDown(child, { key: 'ArrowRight' })
    expect(setExpanded).toHaveBeenCalledTimes(2)
  })
})
