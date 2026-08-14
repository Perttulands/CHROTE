import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  PINNED_STORAGE_KEY,
  readPinnedPaths,
  togglePinnedPath,
  usePinnedPaths,
} from './pinnedPaths'

function PinnedProbe() {
  const [paths] = usePinnedPaths()
  return <output aria-label="Pinned paths">{paths.map(item => item.path).join(',')}</output>
}

describe('pinnedPaths shared store', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
    readPinnedPaths()
  })

  it('returns canonical valid paths when best-effort canonical persistence fails', () => {
    window.localStorage.setItem(PINNED_STORAGE_KEY, JSON.stringify([
      { path: '/srv//chrote/', kind: 'directory' },
      { path: '/srv/chrote', kind: 'directory' },
    ]))
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })

    expect(readPinnedPaths()).toEqual([{ path: '/srv/chrote', kind: 'directory' }])
  })

  it('keeps one authoritative in-memory toggle history when persistence fails', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })

    expect(togglePinnedPath('/srv/chrote', 'directory')).toEqual([{ path: '/srv/chrote', kind: 'directory' }])
    expect(togglePinnedPath('/srv/chrote', 'directory')).toEqual([])
  })

  it('applies cross-tab set, remove, and clear storage events to mounted consumers', () => {
    render(<PinnedProbe />)

    act(() => window.dispatchEvent(new StorageEvent('storage', {
      key: PINNED_STORAGE_KEY,
      newValue: JSON.stringify([{ path: '/srv/chrote', kind: 'directory' }]),
    })))
    expect(screen.getByLabelText('Pinned paths').textContent).toBe('/srv/chrote')

    act(() => window.dispatchEvent(new StorageEvent('storage', {
      key: PINNED_STORAGE_KEY,
      newValue: null,
    })))
    expect(screen.getByLabelText('Pinned paths').textContent).toBe('')

    act(() => window.dispatchEvent(new StorageEvent('storage', {
      key: PINNED_STORAGE_KEY,
      newValue: JSON.stringify([{ path: '/tmp', kind: 'directory' }]),
    })))
    expect(screen.getByLabelText('Pinned paths').textContent).toBe('/tmp')

    act(() => window.dispatchEvent(new StorageEvent('storage', { key: null, newValue: null })))
    expect(screen.getByLabelText('Pinned paths').textContent).toBe('')
  })
})
