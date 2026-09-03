import { describe, expect, it } from 'vitest'
import { parseUnifiedDiff, prettyJson } from './FilePanelViewer'

describe('parseUnifiedDiff', () => {
  it('drops the file headers and signs every changed line in the gutter', () => {
    const rows = parseUnifiedDiff([
      'diff --git a/docs/journeys.md b/docs/journeys.md',
      'index 1111111..2222222 100644',
      '--- a/docs/journeys.md',
      '+++ b/docs/journeys.md',
      '@@ -1,3 +1,3 @@',
      ' kept',
      '-gone',
      '+added',
      '',
    ].join('\n'))

    expect(rows).toEqual([
      { kind: 'hunk', gutter: '', text: '@@ -1,3 +1,3 @@' },
      { kind: 'context', gutter: '', text: 'kept' },
      { kind: 'del', gutter: '-', text: 'gone' },
      { kind: 'add', gutter: '+', text: 'added' },
    ])
  })

  it('keeps a second hunk and the no-newline marker', () => {
    const rows = parseUnifiedDiff('@@ -1 +1 @@\n-a\n@@ -9 +9 @@\n+b\n\\ No newline at end of file\n')

    expect(rows.map(row => row.kind)).toEqual(['hunk', 'del', 'hunk', 'add', 'context'])
    expect(rows[4].text).toBe('\\ No newline at end of file')
  })

  it.each(['', 'diff --git a/x b/x\nindex 1..2 100644\n'])('reads a diff with no hunk as no rows', diff => {
    expect(parseUnifiedDiff(diff)).toEqual([])
  })
})

describe('prettyJson', () => {
  it('pretty-prints what parses', () => {
    expect(prettyJson('{"a":1,"b":[2,3]}')).toBe('{\n  "a": 1,\n  "b": [\n    2,\n    3\n  ]\n}')
  })

  it('shows the bytes on disk when the file is not valid JSON', () => {
    expect(prettyJson('{not json')).toBe('{not json')
  })
})
