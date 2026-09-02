/* The two pieces every session line is made of, wherever a session is listed:
   its name and the mark of what is running in it. A tag, a session row and the
   drag ghost all draw them the same way, so the ghost matches what it was
   dragged from. */

import { HarnessMark, harnessIdForCommand } from './harnessMarks'

/**
 * The name split for tail-preserving truncation: head is everything up to and
 * including the last hyphen, tail is the rest. Names here are prefix-heavy
 * (claude-chrote-architect), so the head is the part worth clipping and the
 * tail is the part that tells two sessions apart.
 */
export function splitSessionName(name: string): { head: string; tail: string } {
  const cut = name.lastIndexOf('-')
  if (cut < 0) return { head: '', tail: name }
  return { head: name.slice(0, cut + 1), tail: name.slice(cut + 1) }
}

interface SessionLabelProps {
  name: string
  /** The call site's own hook, kept so tag and row styling stay where they are. */
  className?: string
}

export function SessionLabel({ name, className }: SessionLabelProps) {
  const { head, tail } = splitSessionName(name)
  return (
    <span className={className ? `session-label ${className}` : 'session-label'} title={name}>
      {head && <span className="session-label-head">{head}</span>}
      <span className="session-label-tail">{tail}</span>
    </span>
  )
}

/**
 * What tmux reports running in the session, as a mark. A known agent gets its
 * product mark; a shell gets nothing, because a shell is the resting state;
 * anything else is named in its own words rather than translated.
 */
export function SessionCommandMark({ command }: { command?: string | null }) {
  const cmd = command?.trim()
  if (!cmd) return null
  const harness = harnessIdForCommand(cmd)
  if (harness === 'shell') return null
  const title = `tmux reports ${cmd}`
  if (!harness) return <span className="harness-command" title={title}>{cmd}</span>
  return (
    <span className="harness-mark" title={title} role="img" aria-label={title}>
      <HarnessMark id={harness} size={14} />
    </span>
  )
}
