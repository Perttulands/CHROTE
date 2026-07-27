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

  it('keeps sidecar mode, pinning, and widths isolated per terminal workspace', () => {
    writeWorkspaceDockState('terminal1', {
      activeSidecar: 'files',
      sidecarPinned: true,
      sessionsWidth: 300,
      filesWidth: 360,
    })

    expect(readWorkspaceDockState('terminal1')).toEqual({
      activeSidecar: 'files',
      sidecarPinned: true,
      sessionsWidth: 300,
      filesWidth: 360,
    })
    expect(readWorkspaceDockState('terminal2')).toEqual(DEFAULT_WORKSPACE_DOCK_STATE)
    expect(JSON.parse(localStorage.getItem('chrote.workspaceDock.v2') || '{}')).toMatchObject({
      version: 2,
      workspaces: {
        terminal1: {
          activeSidecar: 'files',
          sidecarPinned: true,
          sessionsWidth: 300,
          filesWidth: 360,
        },
      },
    })
  })

  it('keeps the pin preference while the sidecar is closed', () => {
    writeWorkspaceDockState('terminal1', {
      activeSidecar: null,
      sidecarPinned: true,
      sessionsWidth: 260,
      filesWidth: 320,
    })

    expect(readWorkspaceDockState('terminal1')).toEqual({
      activeSidecar: null,
      sidecarPinned: true,
      sessionsWidth: 260,
      filesWidth: 320,
    })
  })

  it('migrates the old independent rails into one deterministic pinned sidecar', () => {
    window.localStorage.setItem('chrote.workspaceDock.v1', JSON.stringify({
      version: 1,
      workspaces: {
        terminal1: {
          sessionsCollapsed: true,
          filesCollapsed: false,
          sessionsWidth: 300,
          filesWidth: 360,
        },
        terminal2: {
          sessionsCollapsed: false,
          filesCollapsed: false,
          sessionsWidth: 280,
          filesWidth: 340,
        },
      },
    }))

    expect(readWorkspaceDockState('terminal1')).toEqual({
      activeSidecar: 'files',
      sidecarPinned: true,
      sessionsWidth: 300,
      filesWidth: 360,
    })
    expect(readWorkspaceDockState('terminal2')).toEqual({
      activeSidecar: 'sessions',
      sidecarPinned: true,
      sessionsWidth: 280,
      filesWidth: 340,
    })
  })

  it('migrates the legacy global sidebar collapse only into Terminal 1', () => {
    window.localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: false }))

    expect(readWorkspaceDockState('terminal1')).toMatchObject({ activeSidecar: 'sessions', sidecarPinned: true })
    expect(readWorkspaceDockState('terminal2')).toEqual(DEFAULT_WORKSPACE_DOCK_STATE)
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
