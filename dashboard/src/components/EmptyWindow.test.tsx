import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmptyWindow from './EmptyWindow'
import { ThemeContext } from '../theme/ThemeContext'
import { DEFAULT_SETTINGS } from '../types'
import { DEFAULT_THEME, type Theme } from '../theme/theme'

const createSession = vi.fn()

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: [],
    settings: DEFAULT_SETTINGS,
    terminalUsers: ['alice', 'build'],
    createSession,
  }),
}))

function renderWithTheme(theme: Theme, colorIndex: number) {
  return render(
    <ThemeContext.Provider value={theme}>
      <EmptyWindow workspaceId="terminal1" windowId="terminal1-window-1" colorIndex={colorIndex} />
    </ThemeContext.Provider>,
  )
}

describe('EmptyWindow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
      ok: true,
      json: async () => ({ harnesses: [{ id: 'shell', label: 'Shell' }], folders: ['~'] }),
    })))
  })

  it('stands the launcher on the slot\'s own illustration', async () => {
    const theme: Theme = { ...DEFAULT_THEME, art: ['town.webp', 'crew.webp', 'convoy.webp'] }
    const { container } = renderWithTheme(theme, 4)

    // Slot 4 of three pictures wraps to the second one, so the same tile shows
    // the same picture on every device.
    const art = container.querySelector('.empty-window-art') as HTMLElement
    expect(art.style.backgroundImage).toBe('url("/api/theme/art/crew.webp")')
    expect(await screen.findByRole('button', { name: 'Open shell in home' })).toBeInTheDocument()
    expect(screen.getByText('or drag a session here')).toBeInTheDocument()
  })

  it('is the launcher alone when the theme ships no art', async () => {
    const { container } = renderWithTheme({ ...DEFAULT_THEME, art: [] }, 0)

    expect(container.querySelector('.empty-window-art')).toBeNull()
    expect(await screen.findByRole('button', { name: 'Open shell in home' })).toBeInTheDocument()
  })
})
