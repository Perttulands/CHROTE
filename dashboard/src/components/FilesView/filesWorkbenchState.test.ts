import { beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_FILES_WORKBENCH_STATE,
  readFilesWorkbenchState,
  writeFilesWorkbenchState,
} from './filesWorkbenchState'

describe('Files workbench persistence', () => {
  beforeEach(() => window.localStorage.clear())

  it('restores navigation, open files, active content, tree, widths, and viewer state', () => {
    writeFilesWorkbenchState({
      ...DEFAULT_FILES_WORKBENCH_STATE,
      currentPath: '/srv/chrote',
      history: ['/', '/srv', '/srv/chrote'],
      historyIndex: 2,
      viewMode: 'grid',
      contentMode: 'file',
      expandedPaths: ['/', '/srv', '/srv/chrote'],
      treeScrollTop: 180,
      explorerWidth: 340,
      openFiles: [{ path: '/srv/chrote/README.md', name: 'README.md', size: 100, type: 'text/markdown', kind: 'text' }],
      activeFilePath: '/srv/chrote/README.md',
      fileViewStates: {
        '/srv/chrote/README.md': {
          scrollTop: 420,
          markdownMode: 'split',
          fontSize: 17,
          markdownSplitPercent: 62,
          imageZoom: 1.4,
          imageFit: true,
        },
      },
    })

    expect(readFilesWorkbenchState()).toMatchObject({
      version: 1,
      currentPath: '/srv/chrote',
      historyIndex: 2,
      viewMode: 'grid',
      contentMode: 'file',
      treeScrollTop: 180,
      explorerWidth: 340,
      activeFilePath: '/srv/chrote/README.md',
      openFiles: [{ path: '/srv/chrote/README.md', kind: 'text' }],
      fileViewStates: {
        '/srv/chrote/README.md': { scrollTop: 420, markdownMode: 'split', fontSize: 17 },
      },
    })
  })

  it('drops malformed records and repairs an active path that is not open', () => {
    window.localStorage.setItem('chrote.files.workbench.v1', JSON.stringify({
      currentPath: 42,
      history: ['/', false],
      historyIndex: 999,
      openFiles: [{ path: false }, { path: '/ok.txt', name: 'ok.txt', kind: 'nope' }],
      activeFilePath: '/missing.txt',
      fileViewStates: null,
      explorerWidth: 9000,
    }))

    expect(readFilesWorkbenchState()).toEqual({
      ...DEFAULT_FILES_WORKBENCH_STATE,
      explorerWidth: 560,
    })
  })

  it('rejects records from an unknown future schema version', () => {
    window.localStorage.setItem('chrote.files.workbench.v1', JSON.stringify({
      version: 2,
      currentPath: '/should-not-load',
    }))

    expect(readFilesWorkbenchState()).toEqual(DEFAULT_FILES_WORKBENCH_STATE)
  })
})
