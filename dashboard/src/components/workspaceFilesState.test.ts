import { beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_WORKSPACE_DOCK_STATE,
  DEFAULT_WORKSPACE_FILES_STATE,
  readWorkspaceDockState,
  readWorkspaceFilesState,
  writeWorkspaceDockState,
  writeWorkspaceFilesState,
} from './workspaceFilesState'

describe('workspace Files persistence', () => {
  beforeEach(() => window.localStorage.clear())

  it('keeps dock visibility and widths isolated per terminal workspace', () => {
    writeWorkspaceDockState('terminal1', {
      sessionsCollapsed: true,
      filesCollapsed: false,
      sessionsWidth: 300,
      filesWidth: 360,
    })

    expect(readWorkspaceDockState('terminal1')).toEqual({
      sessionsCollapsed: true,
      filesCollapsed: false,
      sessionsWidth: 300,
      filesWidth: 360,
    })
    expect(readWorkspaceDockState('terminal2')).toEqual(DEFAULT_WORKSPACE_DOCK_STATE)
    expect(JSON.parse(localStorage.getItem('chrote.workspaceDock.v1') || '{}')).toMatchObject({
      version: 1,
      workspaces: {
        terminal1: {
          sessionsCollapsed: true,
          filesCollapsed: false,
          sessionsWidth: 300,
          filesWidth: 360,
        },
      },
    })
  })

  it('migrates the legacy global sidebar collapse only into Terminal 1', () => {
    window.localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: true }))

    expect(readWorkspaceDockState('terminal1').sessionsCollapsed).toBe(true)
    expect(readWorkspaceDockState('terminal2').sessionsCollapsed).toBe(false)
  })

  it('keeps navigation, tree, Peek, and viewer state isolated per terminal workspace', () => {
    writeWorkspaceFilesState('terminal1', {
      ...DEFAULT_WORKSPACE_FILES_STATE,
      currentPath: '/srv/chrote',
      expandedPaths: ['/', '/srv', '/srv/chrote'],
      selectedPath: '/srv/chrote/README.md',
      treeScrollTop: 240,
      peek: {
        path: '/srv/chrote/README.md',
        name: 'README.md',
        size: 100,
        type: 'text/markdown',
        x: 420,
        y: 120,
        width: 720,
        height: 640,
      },
      fileViewStates: {
        '/srv/chrote/README.md': {
          scrollTop: 480,
          markdownMode: 'preview',
          fontSize: 16,
          markdownSplitPercent: 58,
          imageZoom: 1.25,
          imageFit: true,
        },
      },
    })

    expect(readWorkspaceFilesState('terminal1')).toMatchObject({
      currentPath: '/srv/chrote',
      selectedPath: '/srv/chrote/README.md',
      treeScrollTop: 240,
      peek: { path: '/srv/chrote/README.md', width: 720 },
      fileViewStates: {
        '/srv/chrote/README.md': { scrollTop: 480, markdownMode: 'preview' },
      },
    })
    expect(readWorkspaceFilesState('terminal2')).toEqual(DEFAULT_WORKSPACE_FILES_STATE)
  })

  it('sanitizes malformed and stale persisted state instead of trusting it', () => {
    window.localStorage.setItem('chrote.workspaceFiles.v1', JSON.stringify({
      terminal1: {
        currentPath: 42,
        expandedPaths: ['/', 12, '/srv'],
        selectedPath: false,
        treeScrollTop: -9,
        peek: { path: '', width: 99999 },
        fileViewStates: null,
      },
    }))

    expect(readWorkspaceFilesState('terminal1')).toEqual({
      ...DEFAULT_WORKSPACE_FILES_STATE,
      expandedPaths: ['/', '/srv'],
    })
  })
})
