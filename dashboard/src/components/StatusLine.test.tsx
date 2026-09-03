import { act, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import StatusLine from './StatusLine'
import { StatusProvider, useStatus, type StatusSeverity } from '../context/StatusContext'

let announce: (message: string, severity: StatusSeverity) => void

function Announcer() {
  announce = useStatus().announce
  return null
}

function renderLine() {
  return render(
    <StatusProvider>
      <Announcer />
      <StatusLine />
    </StatusProvider>,
  )
}

describe('StatusLine', () => {
  it('is present and 28px tall before anything has been announced', () => {
    const { container } = renderLine()

    const line = container.querySelector('.status-line') as HTMLElement
    expect(line).toBeInTheDocument()
    expect(line.textContent).toBe('')
    expect(screen.getByRole('status')).toBe(line)
  })

  it('carries the last event with its time, newest replacing the one before it', () => {
    const { container } = renderLine()

    act(() => announce('Sent to claude-chrote-ui', 'success'))
    expect(screen.getByRole('status')).toHaveTextContent('Sent to claude-chrote-ui')
    expect(container.querySelector('.status-line-time')?.textContent).toMatch(/\d/)

    act(() => announce('Layout saved', 'info'))
    expect(screen.getByRole('status')).toHaveTextContent('Layout saved')
    expect(screen.queryByText('Sent to claude-chrote-ui')).toBeNull()
  })

  it('marks a failure and leaves every other severity plain', () => {
    const { container } = renderLine()

    act(() => announce('Failed to create session', 'error'))
    expect(container.querySelector('.status-line-message')).toHaveClass('status-line-failure')

    act(() => announce('Session created', 'success'))
    expect(container.querySelector('.status-line-message')).not.toHaveClass('status-line-failure')

    act(() => announce('Harness did not start', 'warning'))
    expect(container.querySelector('.status-line-message')).not.toHaveClass('status-line-failure')
  })
})
