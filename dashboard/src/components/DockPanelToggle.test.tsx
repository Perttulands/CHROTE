import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import DockPanelToggle from './DockPanelToggle'

describe('DockPanelToggle', () => {
  it.each(['Sessions', 'Files'])('renders %s as one compact label-and-chevron control', label => {
    const onToggle = vi.fn()
    const { rerender } = render(<DockPanelToggle label={label} collapsed onToggle={onToggle} />)

    const expand = screen.getByRole('button', { name: `Expand ${label} panel` })
    expect(expand).toHaveClass('dock-toggle-btn')
    expect(expand).toHaveTextContent(`${label}>>`)
    expect(expand.querySelector('.dock-toggle-label')).toHaveTextContent(label)
    expect(expand.querySelector('.dock-toggle-chevron')).toHaveTextContent('>>')
    fireEvent.click(expand)
    expect(onToggle).toHaveBeenCalledOnce()

    rerender(<DockPanelToggle label={label} collapsed={false} onToggle={onToggle} />)
    expect(screen.getByRole('button', { name: `Collapse ${label} panel` })).toHaveTextContent(`${label}<<`)
  })
})
