// The interface's one theme, served by the host.
//
// CHROTE keeps a single active theme, authored on the host and served by
// GET /api/theme. The dashboard never picks one: there is no theme setting and
// no picker, so everything below is a read of what the server says, applied
// once. DEFAULT_THEME is the same JSON the server embeds, so first paint from
// theme-colors.css and a failed fetch both land on exactly these values.

/** Chrome colours. Every entry maps to one CSS custom property on :root. */
export interface ThemeUi {
  background: string
  surface: string
  surfaceRaised: string
  divider: string
  text: string
  textSecondary: string
  textDim: string
  accent: string
  error: string
}

/** The xterm palette. `ansi` is exactly 16 entries, black … brightWhite. */
export interface TerminalTheme {
  background: string
  foreground: string
  cursor: string
  selectionBackground: string
  ansi: string[]
}

export interface Theme {
  schema: 1
  name: string
  ui: ThemeUi
  terminal: TerminalTheme
  /** Per-Unix-user colours, indexed by the server's terminalUsers order. */
  identity: string[]
  /** Art file names served by GET /api/theme/art/{name}. */
  art: string[]
}

/**
 * The one font stack, for chrome and terminal alike. JetBrains Mono carries
 * text; the Iosevka Term subset behind it carries the box drawing, braille and
 * powerline glyphs agent TUIs paint with. Both are served from this origin.
 */
export const TERMINAL_FONT_FAMILY = '"JetBrains Mono", "CHROTE Term Symbols", monospace'

export const DEFAULT_THEME: Theme = {
  schema: 1,
  name: 'chrote-dark',
  ui: {
    background: '#0f0f0f',
    surface: '#1a1a1a',
    surfaceRaised: '#252525',
    divider: '#3a3a3a',
    text: '#e5e5e5',
    textSecondary: '#a3a3a3',
    textDim: '#737373',
    accent: '#6b9fff',
    error: '#f87171',
  },
  terminal: {
    background: '#0a0a0a',
    foreground: '#e5e5e5',
    cursor: '#e5e5e5',
    selectionBackground: '#6b9fff40',
    ansi: [
      '#0f0f0f', '#f87171', '#8bd450', '#e5c07b', '#6b9fff', '#c084fc', '#45d6d6', '#a3a3a3',
      '#737373', '#ff8a8a', '#a6e37a', '#f0d48a', '#8fb5ff', '#d3a4ff', '#7ae2e2', '#ffffff',
    ],
  },
  identity: ['#4f6d8f', '#8f6f3a', '#5f7f5a', '#7a5f8f'],
  art: [],
}

const COLOR_PATTERN = /^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/
const ART_NAME_PATTERN = /^[A-Za-z0-9._-]+$/

function isColor(value: unknown): value is string {
  return typeof value === 'string' && COLOR_PATTERN.test(value)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

const UI_KEYS: (keyof ThemeUi)[] = [
  'background', 'surface', 'surfaceRaised', 'divider',
  'text', 'textSecondary', 'textDim', 'accent', 'error',
]

/**
 * A theme the dashboard can paint, or null. Anything short of the whole
 * contract is nothing: a half-applied palette is worse to look at than the
 * default, and the operator authored this file, so a partial one is a bug he
 * wants to see rather than a gap worth patching.
 */
export function parseTheme(value: unknown): Theme | null {
  if (!isRecord(value) || value.schema !== 1) return null
  if (typeof value.name !== 'string' || value.name === '') return null

  const ui = value.ui
  if (!isRecord(ui) || !UI_KEYS.every(key => isColor(ui[key]))) return null

  const terminal = value.terminal
  if (!isRecord(terminal)) return null
  if (!isColor(terminal.background) || !isColor(terminal.foreground)) return null
  if (!isColor(terminal.cursor) || !isColor(terminal.selectionBackground)) return null
  const ansi = terminal.ansi
  if (!Array.isArray(ansi) || ansi.length !== 16 || !ansi.every(isColor)) return null

  const identity = value.identity
  if (!Array.isArray(identity) || identity.length < 1 || !identity.every(isColor)) return null

  const art = value.art ?? []
  if (!Array.isArray(art) || !art.every(name => typeof name === 'string' && ART_NAME_PATTERN.test(name))) return null

  return {
    schema: 1,
    name: value.name,
    ui: UI_KEYS.reduce((acc, key) => {
      acc[key] = ui[key] as string
      return acc
    }, {} as ThemeUi),
    terminal: {
      background: terminal.background,
      foreground: terminal.foreground,
      cursor: terminal.cursor,
      selectionBackground: terminal.selectionBackground,
      ansi: [...ansi],
    },
    identity: [...identity],
    art: [...art] as string[],
  }
}

/**
 * Read the host's theme, once. There is no retry and no poll: the theme is a
 * file on the host that changes when the operator applies a new one, and a
 * dashboard that missed it is one reload away from having it.
 */
export async function loadTheme(): Promise<Theme> {
  try {
    const response = await fetch('/api/theme', { signal: AbortSignal.timeout(10000) })
    if (!response.ok) {
      console.warn(`Theme request failed (${response.status}); using the built-in theme`)
      return DEFAULT_THEME
    }
    const parsed = parseTheme(await response.json())
    if (!parsed) {
      console.warn('Theme response did not match schema 1; using the built-in theme')
      return DEFAULT_THEME
    }
    return parsed
  } catch (error) {
    console.warn('Theme request failed; using the built-in theme', error)
    return DEFAULT_THEME
  }
}

function rgbChannels(color: string): string {
  const hex = color.slice(1, 7)
  return [0, 2, 4].map(offset => parseInt(hex.slice(offset, offset + 2), 16)).join(', ')
}

/**
 * Write the theme onto :root. These property names are the contract every
 * stylesheet in the dashboard is written against; theme-colors.css holds the
 * same set at DEFAULT_THEME's values so first paint matches.
 */
export function applyTheme(theme: Theme, root: HTMLElement = document.documentElement): void {
  const { ui, terminal, identity } = theme
  const set = (name: string, value: string) => root.style.setProperty(name, value)

  set('--background', ui.background)
  set('--surface-primary', ui.surface)
  set('--surface-secondary', ui.surfaceRaised)
  set('--divider', ui.divider)
  set('--text-primary', ui.text)
  set('--text-secondary', ui.textSecondary)
  set('--text-dim', ui.textDim)
  set('--accent', ui.accent)
  set('--accent-light', `color-mix(in srgb, ${ui.accent} 12%, transparent)`)
  set('--accent-rgb', rgbChannels(ui.accent))
  set('--color-error', ui.error)
  set('--color-error-light', `color-mix(in srgb, ${ui.error} 15%, transparent)`)

  set('--terminal-background', terminal.background)
  set('--terminal-foreground', terminal.foreground)
  terminal.ansi.forEach((color, index) => set(`--ansi-${index}`, color))

  identity.forEach((color, index) => set(`--identity-${index}`, color))
}

/**
 * The colour standing for a Unix user, taken from the user's position in the
 * server's terminalUsers order so the same user is the same colour on every
 * device. A user the server does not list has no identity to stand for, and
 * gets the dim text colour rather than an invented one.
 */
export function identityColorFor(
  user: string | undefined,
  terminalUsers: readonly string[],
  theme: Theme,
): string {
  const name = user?.trim()
  if (!name) return 'var(--text-dim)'
  const index = terminalUsers.indexOf(name)
  if (index < 0 || theme.identity.length === 0) return 'var(--text-dim)'
  return theme.identity[index % theme.identity.length]
}
