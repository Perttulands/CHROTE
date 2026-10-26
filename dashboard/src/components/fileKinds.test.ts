import { describe, expect, it } from 'vitest'
import { getPreviewKind } from './fileKinds'

const markdownFile = {
  path: '/README.md',
  name: 'README.md',
  isDir: false,
  size: 40,
  modified: '2026-01-01T00:00:00Z',
  type: 'text/markdown',
}

describe('getPreviewKind', () => {
  it.each([
    'events.jsonl',
    'records.ndjson',
    'settings.jsonc',
    'schema.graphql',
    'messages.proto',
    'changes.patch',
    'notebook.ipynb',
    'network.har',
    'map.geojson',
    'state.tfstate',
    '.gitignore',
    'service.timer',
  ])('recognizes %s as previewable text', name => {
    expect(getPreviewKind({ ...markdownFile, path: `/${name}`, name, type: name.split('.').pop() || '' })).toBe('text')
  })

  it.each([
    ['photo.avif', 'image'],
    ['favicon.ico', 'image'],
    ['voice.opus', 'audio'],
    ['clip.m4v', 'video'],
    ['movie.ogv', 'video'],
  ] as const)('recognizes %s as a browser-native %s preview', (name, expected) => {
    expect(getPreviewKind({ ...markdownFile, path: `/${name}`, name, type: name.split('.').pop() || '' })).toBe(expected)
  })

  it.each([
    ['go.mod', 'text'],
    ['go.sum', 'text'],
    ['go.work', 'text'],
    ['music.mod', 'download'],
    ['publisher.pub', 'download'],
    ['tiles.map', 'download'],
    ['transport.mts', 'download'],
    ['game.vb', 'download'],
    ['flash-cookie.sol', 'download'],
    ['notability.note', 'download'],
  ] as const)('classifies ambiguous format %s as %s before content probing', (name, expected) => {
    expect(getPreviewKind({ ...markdownFile, path: `/${name}`, name, type: name.split('.').pop() || '' })).toBe(expected)
  })
})
