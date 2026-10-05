import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import FlowView from './FlowView'
import type { WorkRow } from '../../beads/beadsTree'
vi.mock('../../context/TableContext', () => ({ useTableObject: () => null }))
const epic = (path: string, id: string): WorkRow => ({ projectPath: path, projectName: path, id, title: id,
  status: 'open', type: 'epic', priority: 1, blocked: false, linked: true })
const child = (parent: WorkRow): WorkRow => ({ ...parent, id: `${parent.id}.1`, title: 'A child', type: 'task', parent: parent.id })
describe('Flow selection', () => {
  it('chooses the intentionally selected project while retaining a disappeared graph until an explicit choice', () => {
    const first = epic('/first', 'first-one')
    const second = epic('/second', 'second-one')
    const view = render(<FlowView rows={[first, child(first)]} scopeKey="/first" />)
    expect(screen.getByText('first-one · first-one')).toBeVisible()
    view.rerender(<FlowView rows={[second, child(second)]} scopeKey="/second" />)
    expect(screen.getByText('second-one · second-one')).toBeVisible()
    const other = epic('/second', 'second-two')
    view.rerender(<FlowView rows={[other, child(other)]} scopeKey="/second" />)
    expect(screen.getByText(/The selected epic is no longer in open work/)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Choose second-two' }))
    expect(screen.getByText('second-two · second-two')).toBeVisible()
  })
  it('does not describe an incomplete cold scope as having no epics', () => {
    render(<FlowView rows={[]} incomplete />)
    expect(screen.queryByText('No epic here to flow.')).toBeNull()
  })
})
