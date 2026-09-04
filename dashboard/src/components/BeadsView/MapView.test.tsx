import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MapView from './MapView'
import { resetBeadCardForTest } from '../../beads/beadCard'
import type { BeadTreeNode, WorkRow } from '../../beads/beadsTree'

vi.mock('../../context/SessionContext', () => ({
  useSession: () => ({ openSendToSession: vi.fn() }),
}))

vi.mock('../../context/StatusContext', () => ({
  useStatus: () => ({ announce: vi.fn() }),
}))

function row(id: string): WorkRow {
  return { id, title: `Title of ${id}`, status: 'open', type: 'task', priority: 1, blocked: false, linked: false, projectPath: '/srv/chrote', projectName: 'chrote' }
}

function node(id: string, children: BeadTreeNode[] = []): BeadTreeNode {
  return { row: row(id), children }
}

const shown = (id: string) => screen.queryByText(`Title of ${id}`) !== null
const foldOf = (id: string) => screen.getByRole('button', { name: new RegExp(`Title of ${id.replace('.', '\\.')}$`) }).querySelector('.bead-row-fold')

afterEach(() => resetBeadCardForTest())

describe('the map', () => {
  // An order given on a row reaches every row beneath it, and a fold the
  // operator makes himself afterwards is his until the next order.
  it('collapses and expands a whole subtree from the row\'s menu', () => {
    render(<MapView roots={[node('ep', [node('ep.1', [node('ep.1.1')]), node('ep.2')])]} expandAll={false} />)
    expect(shown('ep.1.1')).toBe(true)

    fireEvent.contextMenu(screen.getByText('Title of ep'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Collapse all' }))
    expect(shown('ep.1')).toBe(false)

    // Opening the epic one level shows its children still folded.
    fireEvent.click(screen.getByText('Title of ep'))
    expect(shown('ep.1')).toBe(true)
    expect(shown('ep.1.1')).toBe(false)
    expect(foldOf('ep.1')).toHaveTextContent('▸1')

    fireEvent.contextMenu(screen.getByText('Title of ep'))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Expand all' }))
    expect(shown('ep.1.1')).toBe(true)

    fireEvent.click(screen.getByText('Title of ep.1'))
    expect(shown('ep.1.1')).toBe(false)
  })
})
