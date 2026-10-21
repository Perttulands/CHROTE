/**
 * The one line that names a Bead to an agent: "bead <id>: <title>". The drawer
 * puts it first in a message, a resident's column pastes it into a prompt, and
 * both read the same words so the agent is never asked about something it
 * cannot place. A Bead whose title has not arrived is named by its id alone.
 * When the id alone does not say which store holds the Bead, as when two
 * stores issue the same prefix, the store's folder follows the id.
 */
export function beadReference(bead: { id: string; title?: string }, store?: string): string {
  const title = bead.title?.trim()
  const name = store ? `bead ${bead.id} (${store})` : `bead ${bead.id}`
  return title ? `${name}: ${title}` : name
}
