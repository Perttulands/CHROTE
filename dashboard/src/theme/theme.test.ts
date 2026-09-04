import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_THEME, applyTheme, identityColorFor, loadTheme, type Theme } from './theme'

function themeFixture(overrides: Partial<Theme> = {}): Theme {
  return { ...DEFAULT_THEME, ...overrides }
}

function respondWith(body: unknown, init: { ok?: boolean; status?: number } = {}) {
  return vi.fn().mockResolvedValue({
    ok: init.ok ?? true,
    status: init.status ?? 200,
    json: async () => body,
  })
}

describe('loadTheme', () => {
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('reads the host theme once, with no retry', async () => {
    const served = themeFixture({ name: 'operator-theme', art: ['town.webp'] })
    const fetchMock = respondWith(served)
    vi.stubGlobal('fetch', fetchMock)

    await expect(loadTheme()).resolves.toEqual(served)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/theme')

    vi.unstubAllGlobals()
  })

  it.each([
    ['the request throws', () => vi.fn().mockRejectedValue(new Error('offline'))],
    ['the server refuses', () => respondWith(null, { ok: false, status: 500 })],
    ['the body is not schema 1', () => respondWith({ schema: 2, name: 'next', ui: {} })],
    ['a colour is not a colour', () => respondWith(themeFixture({
      ui: { ...DEFAULT_THEME.ui, accent: 'blue' },
    }))],
    ['the ansi palette is short', () => respondWith(themeFixture({
      terminal: { ...DEFAULT_THEME.terminal, ansi: DEFAULT_THEME.terminal.ansi.slice(0, 8) },
    }))],
    ['identity is empty', () => respondWith(themeFixture({ identity: [] }))],
    ['a shelf hue is not a colour', () => respondWith(themeFixture({ shelves: ['teal'] }))],
    ['an art name could escape its directory', () => respondWith(themeFixture({ art: ['../secret'] }))],
  ])('falls back to the built-in theme when %s', async (_label, makeFetch) => {
    vi.stubGlobal('fetch', makeFetch())

    await expect(loadTheme()).resolves.toEqual(DEFAULT_THEME)

    vi.unstubAllGlobals()
  })
})

// A theme the operator authored before the map had colour names no shelf hues.
// Refusing it would take the whole theme away over a field it could not have
// carried, so the built-in palette answers for that one field.
describe('a theme without shelf hues', () => {
  it('keeps the rest of the theme and takes the built-in palette', async () => {
    const served = { ...themeFixture({ name: 'older-theme' }) } as Partial<Theme>
    delete served.shelves
    vi.stubGlobal('fetch', respondWith(served))

    const theme = await loadTheme()
    expect(theme.name).toBe('older-theme')
    expect(theme.shelves).toEqual(DEFAULT_THEME.shelves)

    vi.unstubAllGlobals()
  })
})

describe('applyTheme', () => {
  afterEach(() => {
    document.documentElement.removeAttribute('style')
  })

  it('writes every custom property the stylesheets are written against', () => {
    const root = document.createElement('div')
    applyTheme(DEFAULT_THEME, root)

    const value = (name: string) => root.style.getPropertyValue(name)

    expect(value('--background')).toBe('#0f0f0f')
    expect(value('--surface-primary')).toBe('#1a1a1a')
    expect(value('--surface-secondary')).toBe('#252525')
    expect(value('--divider')).toBe('#3a3a3a')
    expect(value('--text-primary')).toBe('#e5e5e5')
    expect(value('--text-secondary')).toBe('#a3a3a3')
    expect(value('--text-dim')).toBe('#737373')
    expect(value('--accent')).toBe('#6b9fff')
    expect(value('--accent-light')).toBe('color-mix(in srgb, #6b9fff 12%, transparent)')
    expect(value('--accent-rgb')).toBe('107, 159, 255')
    expect(value('--color-error')).toBe('#f87171')
    expect(value('--color-error-light')).toBe('color-mix(in srgb, #f87171 15%, transparent)')

    expect(value('--terminal-background')).toBe('#0a0a0a')
    expect(value('--terminal-foreground')).toBe('#e5e5e5')
    expect(value('--ansi-0')).toBe('#0f0f0f')
    expect(value('--ansi-15')).toBe('#ffffff')
    expect(Array.from({ length: 16 }, (_, i) => value(`--ansi-${i}`))).toEqual(DEFAULT_THEME.terminal.ansi)

    expect(value('--identity-0')).toBe('#4f6d8f')
    expect(value('--identity-3')).toBe('#7a5f8f')

    expect(value('--shelf-0')).toBe('#b67777')
    expect(Array.from({ length: DEFAULT_THEME.shelves.length }, (_, i) => value(`--shelf-${i}`)))
      .toEqual(DEFAULT_THEME.shelves)
  })

})

describe('identityColorFor', () => {
  const terminalUsers = ['alice', 'bob', 'carol', 'dave', 'erin']

  it('gives each user the identity colour at its position in the server order', () => {
    expect(identityColorFor('alice', terminalUsers, DEFAULT_THEME)).toBe('#4f6d8f')
    expect(identityColorFor('bob', terminalUsers, DEFAULT_THEME)).toBe('#8f6f3a')
    expect(identityColorFor('dave', terminalUsers, DEFAULT_THEME)).toBe('#7a5f8f')
  })

  it('wraps once the users outnumber the identity colours', () => {
    expect(identityColorFor('erin', terminalUsers, DEFAULT_THEME))
      .toBe(identityColorFor('alice', terminalUsers, DEFAULT_THEME))
  })

  it('reads the theme it is handed rather than the built-in one', () => {
    const theme = themeFixture({ identity: ['#111111', '#222222'] })
    expect(identityColorFor('alice', terminalUsers, theme)).toBe('#111111')
    expect(identityColorFor('carol', terminalUsers, theme)).toBe('#111111')
    expect(identityColorFor('bob', terminalUsers, theme)).toBe('#222222')
  })

  // A user the server does not list has no identity to stand for. Inventing one
  // would say a fact CHROTE does not have.
  it.each([
    ['a user the server does not list', 'mallory'],
    ['no user at all', undefined],
    ['whitespace', '   '],
  ])('gives the dim text colour for %s', (_label, user) => {
    expect(identityColorFor(user, terminalUsers, DEFAULT_THEME)).toBe('var(--text-dim)')
  })
})
