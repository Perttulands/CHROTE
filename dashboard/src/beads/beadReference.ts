/**
 * The one line that names a Bead to an agent: "bead <id>: <title>". The drawer
 * puts it first in a message, a resident's column pastes it into a prompt, and
 * both read the same words so the agent is never asked about something it
 * cannot place. A Bead whose title has not arrived is named by its id alone.
 */
export function beadReference(bead: { id: string; title?: string }): string {
  const title = bead.title?.trim()
  return title ? `bead ${bead.id}: ${title}` : `bead ${bead.id}`
}
