import { afterEach, describe, expect, it } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { ILink, Terminal } from '@xterm/xterm'
import { createPathLinkProvider, findPaths } from './pathLinks'
import { resetOpenInFilesForTest, useOpenInFilesRequest } from './openInFiles'
import { resetImageGlanceForTest, useImageGlanceRequest } from '../components/imageGlance'

afterEach(() => {
  resetOpenInFilesForTest()
  resetImageGlanceForTest()
})

describe('absolute paths as terminal links', () => {
  it('covers exactly the path, in the columns xterm counts', () => {
    const links = bufferLinks([{ text: 'wrote /srv/chrote/docs/journeys.md now' }], 80, 1)
    expect(links).toHaveLength(1)
    expect(links[0].text).toBe('/srv/chrote/docs/journeys.md')
    expect(links[0].range).toEqual({ start: { x: 7, y: 1 }, end: { x: 34, y: 1 } })
  })

  it.each([
    ['a sentence ends: see /tmp/out.log.', '/tmp/out.log'],
    ['in brackets (/srv/chrote/README.md)', '/srv/chrote/README.md'],
    ['quoted "/home/alice/notes.txt"', '/home/alice/notes.txt'],
    ['with a line: /srv/chrote/dashboard/src/App.tsx:123:5', '/srv/chrote/dashboard/src/App.tsx'],
    ['a directory /srv/chrote/', '/srv/chrote'],
    ['--flag=/etc/hosts', '/etc/hosts'],
  ])('trims what the sentence added: %s', (line, path) => {
    expect(findPaths(line).map(found => found.path)).toEqual([path])
  })

  it.each([
    'see https://example.com/deep/link for the run',
    'file:///srv/chrote is a URL',
    'either/or, 3/4, and/or 2026/09/03',
    'a bare / is nothing',
    '~/relative and ./relative are not absolute',
  ])('offers no link on: %s', line => {
    expect(findPaths(line)).toEqual([])
  })

  it('hands the path to Files rather than checking it here', () => {
    const { result } = renderHook(() => useOpenInFilesRequest())
    const [link] = bufferLinks([{ text: 'see /var/log/syslog' }], 80, 1)
    act(() => link.activate(new MouseEvent('click'), link.text))
    expect(result.current?.path).toBe('/var/log/syslog')
  })

  // A picture is a file like any other now: Files reads it in the pop-out
  // beside the tree, and the centred glance is not on this way in.
  it('hands a picture to Files too, and opens no glance', () => {
    const files = renderHook(() => useOpenInFilesRequest())
    const glance = renderHook(() => useImageGlanceRequest())
    const [link] = bufferLinks([{ text: 'saved /tmp/shot.PNG' }], 80, 1)
    act(() => link.activate(new MouseEvent('click'), link.text))
    expect(files.result.current?.path).toBe('/tmp/shot.PNG')
    expect(glance.result.current).toBeNull()
  })
})

// A buffer fixture keeps wrap provenance and cell widths explicit; browser
// journeys below this layer exercise xterm's actual link hit-testing.
function bufferLinks(rows: { text: string; wrapped?: boolean }[], columns: number, row: number): ILink[] {
  const lines = rows.map(({ text, wrapped }) => {
    const cells: { chars: string; width: number }[] = []
    for (const char of text) {
      const width = char === '界' ? 2 : 1
      if (/\p{Mark}/u.test(char)) cells[cells.length - 1].chars += char
      else {
        cells.push({ chars: char, width })
        if (width === 2) cells.push({ chars: '', width: 0 })
      }
    }
    while (cells.length < columns) cells.push({ chars: '', width: 1 })
    return {
      isWrapped: wrapped ?? false,
      length: columns,
      translateToString: (trim: boolean) => trim ? text.trimEnd() : text.padEnd(columns),
      getCell: (x: number) => cells[x] && ({
        getChars: () => cells[x].chars,
        getWidth: () => cells[x].width,
      }),
    }
  })
  const terminal = { cols: columns, buffer: { active: {
    length: lines.length, getLine: (y: number) => lines[y],
  } } } as unknown as Terminal
  let result: ILink[] = []
  createPathLinkProvider(terminal).provideLinks(row, links => { result = links ?? [] })
  return result
}

describe('wrapped terminal paths', () => {
  it('opens the whole soft-wrapped path from either row', () => {
    const rows = [{ text: 'see /tmp/abc' }, { text: 'def.png now', wrapped: true }]
    for (const row of [1, 2]) {
      const [link] = bufferLinks(rows, 12, row)
      expect(link.text).toBe('/tmp/abcdef.png')
      expect(link.range).toEqual(row === 1
        ? { start: { x: 5, y: 1 }, end: { x: 12, y: 1 } }
        : { start: { x: 1, y: 2 }, end: { x: 7, y: 2 } })
    }
  })

  it('reconstructs an enclosed application wrap without linking indentation', () => {
    const rows = [
      { text: '  saved (/tmp/evid' },
      { text: '  ence/shot.png).' },
    ]
    for (const row of [1, 2]) {
      const [link] = bufferLinks(rows, 18, row)
      expect(link.text).toBe('/tmp/evidence/shot.png')
      expect(link.range).toEqual(row === 1
        ? { start: { x: 10, y: 1 }, end: { x: 18, y: 1 } }
        : { start: { x: 3, y: 2 }, end: { x: 15, y: 2 } })
      const files = renderHook(() => useOpenInFilesRequest())
      act(() => link.activate(new MouseEvent('click'), link.text))
      expect(files.result.current?.path).toBe('/tmp/evidence/shot.png')
      files.unmount()
    }
  })

  it.each([
    ['  saved /tmp/evid', '  ence/shot.png)'], // no opening delimiter
    ['  see (/tmp/evid', '  ence/shot.png)'], // short first row
    ['  saved (/tmp/evid', ' ence/shot.png)'], // different indentation
    ['  saved (/tmp/evid', '  ence/shot.png]'], // mismatched delimiter
    ['  saved (/tmp/evid', '  is a directory)'], // prose
    ['  saved (/tmp/evid', '  ence/shot.png'], // no closing delimiter
  ])('keeps ambiguous hard breaks separate: %s', (first, second) => {
    const links = bufferLinks([{ text: first }, { text: second }], 18, 1)
    expect(links.map(link => link.text)).toEqual(['/tmp/evid'])
  })

  it('uses terminal cells after wide and combining characters', () => {
    const [link] = bufferLinks([{ text: '界界e\u0301 /tmp/a' }], 20, 1)
    expect(link.range).toEqual({ start: { x: 7, y: 1 }, end: { x: 12, y: 1 } })
  })

  it('preserves spaces at a soft wrap and keeps URLs with their own provider', () => {
    expect(bufferLinks([{ text: '/tmp/a  ' }, { text: 'next', wrapped: true }], 8, 1)[0].text).toBe('/tmp/a')
    expect(bufferLinks([{ text: 'https://ex' }, { text: '.com/path', wrapped: true }], 10, 2)).toEqual([])
  })
})

// Application wrapping may split directly after a slash and span several rows.
it('reconstructs a quoted path across three indented rows', () => {
  const rows = [
    { text: '  see "/tmp/' },
    { text: '  long/paths' },
    { text: '  /out.png"' },
  ]
  for (const row of [1, 2, 3]) {
    expect(bufferLinks(rows, 12, row).map(link => link.text)).toEqual(['/tmp/long/paths/out.png'])
  }
})
