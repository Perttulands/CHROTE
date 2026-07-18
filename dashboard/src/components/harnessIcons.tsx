/* Harness icon library: product marks for the agent harnesses CHROTE can drive.
   Rendered wherever an agent coin/face appears (Formations roster, slot faces,
   drag ghosts). Marks inherit currentColor so callers control the tint — except
   Claude Code, which keeps its native terracotta so the critter is recognisable
   (explicit product direction, 2026-07-17).

   Sources: Claude Code pixel critter + Codex blob + OpenCode frame are the
   official product marks (LobeHub / Simple Icons packaging; trademarks belong
   to their owners). Pi and Hermes have no published mark yet — the glyphs here
   are house stand-ins until official assets exist. */

export type HarnessId = 'claude-code' | 'codex' | 'opencode' | 'pi' | 'hermes' | 'terminal'

const CLAUDE_CODE_SVG = (
  <svg viewBox="0 0 24 24" fill="#D97757" fillRule="evenodd" aria-hidden="true">
    <path clipRule="evenodd" d="M20.998 10.949H24v3.102h-3v3.028h-1.487V20H18v-2.921h-1.487V20H15v-2.921H9V20H7.488v-2.921H6V20H4.487v-2.921H3V14.05H0V10.95h3V5h17.998v5.949zM6 10.949h1.488V8.102H6v2.847zm10.51 0H18V8.102h-1.49v2.847z" />
  </svg>
)

const CODEX_SVG = (
  <svg viewBox="0 0 24 24" fill="currentColor" fillRule="evenodd" aria-hidden="true">
    <path clipRule="evenodd" d="M8.086.457a6.105 6.105 0 013.046-.415c1.333.153 2.521.72 3.564 1.7a.117.117 0 00.107.029c1.408-.346 2.762-.224 4.061.366l.063.03.154.076c1.357.703 2.33 1.77 2.918 3.198.278.679.418 1.388.421 2.126a5.655 5.655 0 01-.18 1.631.167.167 0 00.04.155 5.982 5.982 0 011.578 2.891c.385 1.901-.01 3.615-1.183 5.14l-.182.22a6.063 6.063 0 01-2.934 1.851.162.162 0 00-.108.102c-.255.736-.511 1.364-.987 1.992-1.199 1.582-2.962 2.462-4.948 2.451-1.583-.008-2.986-.587-4.21-1.736a.145.145 0 00-.14-.032c-.518.167-1.04.191-1.604.185a5.924 5.924 0 01-2.595-.622 6.058 6.058 0 01-2.146-1.781c-.203-.269-.404-.522-.551-.821a7.74 7.74 0 01-.495-1.283 6.11 6.11 0 01-.017-3.064.166.166 0 00.008-.074.115.115 0 00-.037-.064 5.958 5.958 0 01-1.38-2.202 5.196 5.196 0 01-.333-1.589 6.915 6.915 0 01.188-2.132c.45-1.484 1.309-2.648 2.577-3.493.282-.188.55-.334.802-.438.286-.12.573-.22.861-.304a.129.129 0 00.087-.087A6.016 6.016 0 015.635 2.31C6.315 1.464 7.132.846 8.086.457zm-.804 7.85a.848.848 0 00-1.473.842l1.694 2.965-1.688 2.848a.849.849 0 001.46.864l1.94-3.272a.849.849 0 00.007-.854l-1.94-3.393zm5.446 6.24a.849.849 0 000 1.695h4.848a.849.849 0 000-1.696h-4.848z" />
  </svg>
)

const OPENCODE_SVG = (
  <svg viewBox="0 0 24 24" fill="currentColor" fillRule="evenodd" aria-hidden="true">
    <path d="M16 6H8v12h8V6zm4 16H4V2h16v20z" />
  </svg>
)

/* House stand-in: the pi letterform, drawn to read at 16px. */
const PI_SVG = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" aria-hidden="true">
    <path d="M4 7.2c1.6-.9 4-1.2 16-.9" />
    <path d="M9 6.8V19" />
    <path d="M15.6 6.8V15.6a3.2 3.2 0 003.4 3.3" />
  </svg>
)

/* House stand-in: a swept wing for Hermes. */
const HERMES_SVG = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden="true">
    <path d="M20.5 4C12.5 4.8 6.8 9.8 4 19.5" />
    <path d="M20.5 4c-1.2 3.6-3.6 6.2-7.3 7.8" />
    <path d="M4 19.5c4.4-.9 8.6-2.7 11.6-6.2" />
  </svg>
)

const TERMINAL_SVG = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
    <path d="M4 17l6-5-6-5M12 19h8" />
  </svg>
)

export const HARNESS_ICONS: Record<HarnessId, JSX.Element> = {
  'claude-code': CLAUDE_CODE_SVG,
  codex: CODEX_SVG,
  opencode: OPENCODE_SVG,
  pi: PI_SVG,
  hermes: HERMES_SVG,
  terminal: TERMINAL_SVG,
}

export function harnessIdFor(harness: string | undefined | null): HarnessId | null {
  const h = (harness || '').toLowerCase()
  if (!h) return null
  if (h.includes('opencode') || h.includes('open-code')) return 'opencode'
  if (h.includes('claude') || h.includes('anthropic')) return 'claude-code'
  if (h.includes('codex') || h.includes('openai') || h.includes('gpt')) return 'codex'
  if (h.includes('hermes')) return 'hermes'
  // "pi" is too short for substring matching (would hit "api", "pipeline"): match tokens.
  if (h.split(/[^a-z0-9]+/).includes('pi')) return 'pi'
  return 'terminal'
}

export function harnessIcon(harness: string | undefined | null): JSX.Element | null {
  const id = harnessIdFor(harness)
  return id ? HARNESS_ICONS[id] : null
}
