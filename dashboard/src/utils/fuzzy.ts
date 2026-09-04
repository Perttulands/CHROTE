/**
 * A small fuzzy matcher for picking a path by a few typed letters.
 *
 * The query is words; every word must be found in the candidate as a
 * subsequence, and the best placement of each word wins: a letter that opens
 * a word or follows the previous match is worth more than one found anywhere,
 * and the letters skipped between matches cost a little. Scores compare only
 * within one query, which is all a ranked list needs. No dependency: the
 * candidates are the last two segments of a path, and there are dozens of
 * them, not thousands.
 */

// A run of matched letters is worth more than the same letters found apart:
// the gap costs more than a word start earns, so "chrote" is srv/chrote before
// it is srv/chroma-tester.
const MATCH = 1
const CONSECUTIVE = 3
const WORD_START = 3
const GAP = 1.5

/** The last segments of a path, which is what the operator types towards. */
export function lastSegments(path: string, count = 2): string {
  const parts = path.replace(/\/+$/, '').split('/').filter(Boolean)
  return parts.slice(-count).join('/')
}

function isWordStart(text: string, index: number): boolean {
  if (index === 0) return true
  const previous = text[index - 1]
  if (!/[a-z0-9]/i.test(previous)) return true
  // camelCase: a capital after a lower-case letter opens a word.
  return /[a-z]/.test(previous) && /[A-Z]/.test(text[index])
}

/** The best score for one word, or 0 when the word is not in the text. */
function scoreWord(word: string, text: string, lowered: string): number {
  // best[i][j]: the best score placing word[i..] into text[j..], given the
  // previous letter matched at j - 1 (consecutive) or earlier (not).
  const memo = new Map<string, number>()
  const best = (i: number, from: number, consecutiveAt: number, firstAt: number): number => {
    if (i === word.length) {
      const last = from - 1
      return -(last - firstAt + 1 - word.length) * GAP
    }
    const key = `${i}:${from}:${consecutiveAt}:${firstAt}`
    const known = memo.get(key)
    if (known !== undefined) return known
    let top = Number.NEGATIVE_INFINITY
    for (let index = lowered.indexOf(word[i], from); index !== -1; index = lowered.indexOf(word[i], index + 1)) {
      let score = MATCH
      if (index === consecutiveAt) score += CONSECUTIVE
      if (isWordStart(text, index)) score += WORD_START
      const rest = best(i + 1, index + 1, index + 1, firstAt === -1 ? index : firstAt)
      if (rest === Number.NEGATIVE_INFINITY) continue
      top = Math.max(top, score + rest)
    }
    memo.set(key, top)
    return top
  }
  const score = best(0, 0, -1, -1)
  return score === Number.NEGATIVE_INFINITY ? 0 : Math.max(score, 0.1)
}

/**
 * How well a candidate answers a query: 0 when a word of the query is not in
 * it, higher the more of the query opens words and runs unbroken. An empty
 * query matches everything equally.
 */
export function fuzzyScore(query: string, candidate: string): number {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return 1
  const lowered = candidate.toLowerCase()
  let total = 0
  for (const word of words) {
    const score = scoreWord(word, candidate, lowered)
    if (score === 0) return 0
    total += score
  }
  return total
}

/** The items that match, best first; ties keep the order they came in. */
export function rankByFuzzy<T>(query: string, items: readonly T[], text: (item: T) => string): T[] {
  return items
    .map((item, index) => ({ item, index, score: fuzzyScore(query, text(item)) }))
    .filter(entry => entry.score > 0)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map(entry => entry.item)
}
