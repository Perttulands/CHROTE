/* The flags line as a value: one string the operator can type freely, which
   the catalogue panel edits by name rather than by cursor position. Nothing
   here runs a command; the line is composed on the server, and these functions
   only agree with it about where a token starts and ends. */

/** One flag the host's `--help` reported, as `GET /api/launch` passes it on. */
export interface LaunchFlag {
  /** The first long form, e.g. `--model`. */
  name: string
  /** A short form, when the help printed one, e.g. `-m`. */
  short?: string
  /** The placeholder exactly as help printed it, e.g. `<model>`. Absent means the flag is boolean. */
  value?: string
  description: string
  /** The `[possible values: …]` list, when help printed one. */
  values?: string[]
}

interface Token {
  /** The token with its quotes taken off: what the shell would see. */
  text: string
  /** The token as written, quotes and all: what goes back into the line. */
  raw: string
}

/**
 * The line split the way a shell would split it, single and double quotes
 * grouping whitespace. Each token is kept twice so an edit can put the
 * untouched ones back exactly as the operator wrote them.
 */
function scan(line: string): Token[] {
  const tokens: Token[] = []
  let text = ''
  let raw = ''
  let started = false
  let quote: '"' | "'" | null = null

  const flush = () => {
    if (started) tokens.push({ text, raw })
    text = ''
    raw = ''
    started = false
  }

  for (const char of line) {
    if (quote) {
      raw += char
      if (char === quote) quote = null
      else text += char
      continue
    }
    if (char === '"' || char === "'") {
      started = true
      quote = char
      raw += char
      continue
    }
    if (/\s/.test(char)) {
      flush()
      continue
    }
    started = true
    text += char
    raw += char
  }
  flush()
  return tokens
}

/** The line's tokens, quotes honoured and removed. */
export function tokenize(line: string): string[] {
  return scan(line).map(token => token.text)
}

/** Does this token name the flag: as its long form, its short form, or `--name=value`? */
function names(token: string, entry: LaunchFlag): boolean {
  return token === entry.name || (entry.short !== undefined && token === entry.short) ||
    token.startsWith(`${entry.name}=`)
}

/** Is the flag on the line already? */
export function hasFlag(line: string, entry: LaunchFlag): boolean {
  return scan(line).some(token => names(token.text, entry))
}

/** A value goes back on the line as the operator would have to type it. */
function quoted(value: string): string {
  return /\s/.test(value) ? `"${value}"` : value
}

/**
 * The flag appended to the line, with its value when it takes one. The rest of
 * the line survives as written, separated by single spaces.
 */
export function addFlag(line: string, entry: LaunchFlag, value?: string): string {
  const parts = scan(line).map(token => token.raw)
  parts.push(entry.name)
  if (entry.value !== undefined && value !== undefined && value !== '') parts.push(quoted(value))
  return parts.join(' ')
}

/**
 * The flag taken off the line: the naming token, and for a value flag the
 * token that carried its value. `--name=value` goes in one piece. The rest of
 * the line survives as written, separated by single spaces.
 */
export function removeFlag(line: string, entry: LaunchFlag): string {
  const tokens = scan(line)
  const index = tokens.findIndex(token => names(token.text, entry))
  if (index === -1) return tokens.map(token => token.raw).join(' ')
  const takesSeparateValue = entry.value !== undefined && !tokens[index].text.startsWith(`${entry.name}=`)
  const removed = takesSeparateValue && index + 1 < tokens.length ? 2 : 1
  const kept = [...tokens.slice(0, index), ...tokens.slice(index + removed)]
  return kept.map(token => token.raw).join(' ')
}

/** What a flag's row reads as when the panel lists it: `-m, --model`. */
export function flagNames(entry: LaunchFlag): string {
  return entry.short ? `${entry.short}, ${entry.name}` : entry.name
}

/** The value already on the line for a value flag, or '' when there is none. */
export function flagValue(line: string, entry: LaunchFlag): string {
  const tokens = scan(line)
  const index = tokens.findIndex(token => names(token.text, entry))
  if (index === -1) return ''
  const token = tokens[index].text
  if (token.startsWith(`${entry.name}=`)) return token.slice(entry.name.length + 1)
  return index + 1 < tokens.length ? tokens[index + 1].text : ''
}
