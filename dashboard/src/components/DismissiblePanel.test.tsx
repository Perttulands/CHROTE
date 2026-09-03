import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DismissiblePanel from './DismissiblePanel'
import { resetSurfacesForTest } from '../keys/dismiss'

afterEach(() => {
  resetSurfacesForTest()
})

describe('DismissiblePanel', () => {
  it('lifts a fixed panel to the document root and closes it on a press outside or on Escape', () => {
    const onDismiss = vi.fn()
    const { container } = render(
      <DismissiblePanel onDismiss={onDismiss} panelPosition="fixed">
        <div data-testid="panel-content">Panel</div>
      </DismissiblePanel>
    )

    const panel = screen.getByTestId('panel-content')
    expect(panel.style.position).toBe('fixed')
    expect(container).not.toContainElement(panel)
    expect(document.body).toContainElement(panel)

    fireEvent.pointerDown(panel)
    expect(onDismiss).not.toHaveBeenCalled()

    fireEvent.pointerDown(document.body)
    expect(onDismiss).toHaveBeenCalledTimes(1)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onDismiss).toHaveBeenCalledTimes(2)
  })

  it('keeps absolute dropdowns in their positioning container', () => {
    const { container } = render(
      <DismissiblePanel onDismiss={vi.fn()} panelPosition="absolute">
        <div data-testid="anchored-panel">Panel</div>
      </DismissiblePanel>
    )

    expect(container).toContainElement(screen.getByTestId('anchored-panel'))
  })
})
