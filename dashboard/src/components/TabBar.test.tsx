import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TabBar from './TabBar'
import { setFeatureEnabled } from '../featureFlags'

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

describe('TabBar Missions rename flag', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockMatchMedia(false)
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('labels the formations tab "Formations" when the missionsTab flag is off (default)', () => {
    const onTabChange = vi.fn()
    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)

    // The flag is reversible and defaults off, so the legacy label survives.
    expect(screen.getByRole('button', { name: 'Formations' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Missions' })).toBeNull()
  })

  it('labels the same tab "Missions" when the flag is on and still routes to the formations view', () => {
    setFeatureEnabled('missionsTab', true)
    const onTabChange = vi.fn()
    render(<TabBar activeTab="terminal1" onTabChange={onTabChange} />)

    // Rename only — the tab id stays "formations" so the keep-alive host is unchanged.
    expect(screen.queryByRole('button', { name: 'Formations' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Missions' }))
    expect(onTabChange).toHaveBeenCalledWith('formations')
  })
})
