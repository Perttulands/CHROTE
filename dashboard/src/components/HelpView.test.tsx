import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import HelpView from './HelpView'

describe('HelpView', () => {
  // The help view is the only place the dashboard explains itself, so every
  // section it advertises has to be reachable and has to render its own page.
  it('names itself and shows each advertised section from its nav', () => {
    render(<HelpView />)

    expect(screen.getByRole('heading', { level: 1, name: 'Dashboard Help' })).toBeInTheDocument()
    expect(screen.getByText('How to use this interface.')).toBeInTheDocument()

    // It opens on Shortcuts, and the nav offers the other four.
    expect(screen.getByRole('heading', { level: 2, name: 'Keyboard Shortcuts' })).toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(5)

    for (const [section, heading] of [
      ['Terminals', 'Terminal Panes'],
      ['Sessions', 'Session Sidecar'],
      ['Files', 'File Browser'],
      ['tmux', 'tmux Reference'],
      ['Shortcuts', 'Keyboard Shortcuts'],
    ] as const) {
      fireEvent.click(screen.getByRole('button', { name: new RegExp(`${section}$`) }))

      expect(screen.getByRole('heading', { level: 2, name: heading })).toBeInTheDocument()
      // One section at a time: the previous page is gone, not stacked below.
      expect(screen.getAllByRole('heading', { level: 2 })).toHaveLength(1)
    }
  })
})
