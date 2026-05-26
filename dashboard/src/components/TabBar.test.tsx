import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TabBar from './TabBar'

function mockMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
}

describe('TabBar Services navigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
  })

  it('shows Services in desktop navigation', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Services' }))

    expect(onTabChange).toHaveBeenCalledWith('services')
  })

  it('shows Services in mobile navigation', () => {
    mockMatchMedia(true)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: '☰' }))
    fireEvent.click(screen.getByRole('button', { name: 'Services' }))

    expect(onTabChange).toHaveBeenCalledWith('services')
  })

  it('shows the feature-flagged Server tab in desktop navigation', () => {
    mockMatchMedia(false)
    const onTabChange = vi.fn()

    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)
    fireEvent.click(screen.getByRole('button', { name: 'Server' }))

    expect(onTabChange).toHaveBeenCalledWith('server')
  })

  it('hides the Server tab when the feature flag is disabled', () => {
    mockMatchMedia(false)
    window.localStorage.setItem('chrote-server-status-tab', '0')

    render(<TabBar activeTab="terminal1" onTabChange={vi.fn()} />)

    expect(screen.queryByRole('button', { name: 'Server' })).not.toBeInTheDocument()
  })
})
