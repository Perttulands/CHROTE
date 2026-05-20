import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TabBar from './TabBar'

vi.mock('./MusicPlayer', () => ({
  default: () => <div data-testid="music-player" />,
}))

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
})
