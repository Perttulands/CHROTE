import { afterEach, describe, expect, it } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { findPaths, isImagePath, pathLinksOnLine } from './pathLinks'
import { resetOpenInFilesForTest, useOpenInFilesRequest } from './openInFiles'
import { resetImageGlanceForTest, useImageGlanceRequest } from '../components/imageGlance'

afterEach(() => {
  resetOpenInFilesForTest()
  resetImageGlanceForTest()
})

describe('absolute paths as terminal links', () => {
  it('covers exactly the path, in the columns xterm counts', () => {
    const links = pathLinksOnLine('wrote /srv/chrote/docs/journeys.md now', 7)
    expect(links).toHaveLength(1)
    expect(links[0].text).toBe('/srv/chrote/docs/journeys.md')
    expect(links[0].range).toEqual({ start: { x: 7, y: 7 }, end: { x: 34, y: 7 } })
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
    const [link] = pathLinksOnLine('see /var/log/syslog', 3)
    act(() => link.activate(new MouseEvent('click'), link.text))
    expect(result.current?.path).toBe('/var/log/syslog')
  })

  it('hands a picture to the image glance instead', () => {
    const files = renderHook(() => useOpenInFilesRequest())
    const glance = renderHook(() => useImageGlanceRequest())
    const [link] = pathLinksOnLine('saved /tmp/shot.PNG', 3)
    act(() => link.activate(new MouseEvent('click'), link.text))
    expect(glance.result.current?.path).toBe('/tmp/shot.PNG')
    expect(files.result.current).toBeNull()
  })
})

describe('the picture rule', () => {
  it.each(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'])('shows a .%s', extension => {
    expect(isImagePath(`/srv/evidence/frame.${extension}`)).toBe(true)
  })

  it.each(['/tmp/notes.txt', '/srv/chrote/png', '/srv/x/.png', '/srv/x/icon.ico', '/srv/x/photo.png.txt'])('opens %s in Files', path => {
    expect(isImagePath(path)).toBe(false)
  })
})
