import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import DismissiblePanel from './DismissiblePanel'

describe('DismissiblePanel', () => {
  it('keeps the panel above its sibling layer and dismisses outside or on Escape without swallowing inside interaction', () => {
    const onDismiss = vi.fn()
    const { container } = render(
      <DismissiblePanel onDismiss={onDismiss} panelPosition="fixed">
        <div data-testid="panel-content">Panel</div>
      </DismissiblePanel>
    )

    const panel = screen.getByTestId('panel-content')
    const layer = container.querySelector('.floating-panel-dismiss-layer') as HTMLElement
    expect(panel.style.position).toBe('fixed')
    expect(Number(panel.style.zIndex)).toBeGreaterThan(Number(layer.style.zIndex))

    fireEvent.pointerDown(panel)
    expect(onDismiss).not.toHaveBeenCalled()

    fireEvent.pointerDown(layer)
    expect(onDismiss).toHaveBeenCalledTimes(1)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onDismiss).toHaveBeenCalledTimes(2)
    expect(container).toContainElement(panel)
  })
})
