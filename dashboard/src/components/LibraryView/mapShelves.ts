/**
 * Which colour a shelf is.
 *
 * The map was monochrome, and a corpus of dozens reads that way; a corpus of
 * hundreds does not, because the one thing the eye wants first — which part of
 * the library am I looking at — is exactly what a grey dot cannot say. So each
 * shelf takes one hue, and the hue says the shelf and nothing else. That is
 * what the colour rule allows: colour marks meaning.
 *
 * The hues come from the served theme, so a host that themes CHROTE differently
 * recolours the map with it. They are taken in shelf order, which is the order
 * the rail and the map both read shelves in, so a corpus keeps its colours
 * across a reload and across devices, and a new shelf takes the next hue rather
 * than reshuffling the ones already learned. More shelves than hues is allowed
 * and repeats from the start, which the theme's own check warns about.
 */

/** The shelves of a corpus, in the one order every surface reads them in. */
export function shelfOrder(shelves: Iterable<string>): string[] {
  return Array.from(new Set(Array.from(shelves).filter(name => name !== ''))).sort()
}

/**
 * A hue for each shelf, in shelf order. An empty palette gives an empty map,
 * and the map draws in its own greys.
 */
export function shelfHues(shelves: Iterable<string>, palette: readonly string[]): Map<string, string> {
  const hues = new Map<string, string>()
  if (palette.length === 0) return hues
  shelfOrder(shelves).forEach((shelf, index) => hues.set(shelf, palette[index % palette.length]))
  return hues
}
