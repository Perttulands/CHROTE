import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, extname, isAbsolute, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { ToastProvider } from '../context/ToastContext'

function mockMatchMedia(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
}

function mockFetch() {
  ;(globalThis as Record<string, unknown>).fetch = vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    if (url === '/api/tmux/sessions') {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
        text: () => Promise.resolve(''),
      })
    }
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({}),
      text: () => Promise.resolve(''),
    })
  }) as any
}

function mockResizeObserver() {
  class ResizeObserverMock {
    observe = vi.fn()
    unobserve = vi.fn()
    disconnect = vi.fn()
  }

  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
}

describe('dashboard terminal baseline', () => {
  beforeEach(() => {
    localStorage.clear()
    mockMatchMedia(false)
    mockResizeObserver()
    mockFetch()
  })

  it('renders the stable sidecar switcher with both panels closed when uiV2 is unset', async () => {
    localStorage.removeItem('chrote-ui-v2')
    const { default: App } = await import('../App')

    const { container } = render(
      <ToastProvider>
        <App />
      </ToastProvider>
    )

    expect(await screen.findByRole('button', { name: 'Sessions sidecar' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Files sidecar' })).toBeInTheDocument()
    expect(container.querySelector('.session-panel')).toBeNull()
    expect(container.querySelector('.terminal-files-panel')).toBeNull()
    expect(container.querySelector('.sp-action-btn')).toBeNull()
  })
})

describe('FilesView extracted components import boundary', () => {
  const srcRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
  const filesViewComponentsDir = join(srcRoot, 'components', 'FilesView', 'components')

  function walkSourceFiles(dir: string): string[] {
    return readdirSync(dir).flatMap(entry => {
      const fullPath = join(dir, entry)
      const stats = statSync(fullPath)
      if (stats.isDirectory()) return walkSourceFiles(fullPath)
      if (!/\.(ts|tsx)$/.test(entry)) return []
      return [fullPath]
    })
  }

  function isInsideFilesViewComponents(path: string): boolean {
    const rel = relative(filesViewComponentsDir, path)
    return rel === '' || (!rel.startsWith('..') && !isAbsolute(rel))
  }

  function resolveRelativeImport(sourceFile: string, specifier: string): string | null {
    if (!specifier.startsWith('.')) return null

    const base = resolve(dirname(sourceFile), specifier)
    const candidates = extname(base)
      ? [base]
      : [
          `${base}.ts`,
          `${base}.tsx`,
          `${base}.js`,
          `${base}.jsx`,
          join(base, 'index.ts'),
          join(base, 'index.tsx'),
        ]

    return candidates.find(candidate => existsSync(candidate)) ?? null
  }

  function importSpecifiers(source: string): string[] {
    const importPattern = /\bimport\s+(?:type\s+)?(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]|import\(\s*['"]([^'"]+)['"]\s*\)|\bexport\s+(?:type\s+)?(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]/gs
    return Array.from(source.matchAll(importPattern), match => match[1] ?? match[2] ?? match[3])
  }

  it('has zero importers outside dashboard/src/components/FilesView/components', () => {
    const offenders = walkSourceFiles(srcRoot)
      .filter(file => !isInsideFilesViewComponents(file))
      .flatMap(file => {
        const source = readFileSync(file, 'utf8')
        return importSpecifiers(source).flatMap(specifier => {
          const resolved = resolveRelativeImport(file, specifier)
          if (!resolved || !isInsideFilesViewComponents(resolved)) return []
          return `${relative(srcRoot, file)} -> ${specifier}`
        })
      })

    expect(offenders).toEqual([])
  })
})
