import { beforeEach, describe, expect, it } from 'vitest'
import {
  DEFAULT_SESSIONS_DOCK_STATE,
  DEFAULT_WORKSPACE_FILES_DOCK_STATE,
  DEFAULT_WORKSPACE_FILES_STATE,
  readSessionsDockState,
  readWorkspaceFilesDockState,
  readWorkspaceFilesState,
  writeSessionsDockState,
  writeWorkspaceFilesDockState,
  writeWorkspaceFilesState,
} from './workspaceFilesState'

describe('workspace Files persistence', () => {
  beforeEach(() => window.localStorage.clear())

  it('stores one Sessions presentation with no terminal workspace owner', () => {
    const state = {
      open: true,
      pinned: true,
      width: 300,
      searchTerm: 'hq',
      collapsedGroups: ['hq', 'codex'],
    }
    writeSessionsDockState(state)

    expect(readSessionsDockState()).toEqual(state)
    expect(JSON.parse(localStorage.getItem('chrote.sessionsDock.v1') || '{}')).toEqual({
      version: 1,
      state,
    })
  })

  it('keeps Files presentation isolated per terminal workspace', () => {
    writeWorkspaceFilesDockState('terminal1', { open: true, pinned: true, width: 360 })

    expect(readWorkspaceFilesDockState('terminal1')).toEqual({ open: true, pinned: true, width: 360 })
    expect(readWorkspaceFilesDockState('terminal2')).toEqual(DEFAULT_WORKSPACE_FILES_DOCK_STATE)
  })

  it('keeps independent Sessions and Files pin preferences while either panel is closed', () => {
    writeSessionsDockState({ ...DEFAULT_SESSIONS_DOCK_STATE, open: false, pinned: true, width: 280 })
    writeWorkspaceFilesDockState('terminal1', { open: false, pinned: true, width: 340 })

    expect(readSessionsDockState()).toEqual({ ...DEFAULT_SESSIONS_DOCK_STATE, open: false, pinned: true, width: 280 })
    expect(readWorkspaceFilesDockState('terminal1')).toEqual({ open: false, pinned: true, width: 340 })
  })

  it('migrates only Files presentation from the former per-workspace dock state', () => {
    window.localStorage.setItem('chrote.workspaceDock.v2', JSON.stringify({
      version: 2,
      workspaces: {
        terminal1: {
          openSidecars: ['sessions', 'files'],
          sidecarPinned: true,
          sessionsWidth: 300,
          filesWidth: 360,
        },
        terminal2: {
          activeSidecar: 'sessions',
          sidecarPinned: false,
          sessionsWidth: 280,
          filesWidth: 340,
        },
      },
    }))

    expect(readWorkspaceFilesDockState('terminal1')).toEqual({ open: true, pinned: true, width: 360 })
    expect(readWorkspaceFilesDockState('terminal2')).toEqual({ open: false, pinned: false, width: 340 })
    expect(readSessionsDockState()).toEqual(DEFAULT_SESSIONS_DOCK_STATE)
  })

  it('never lets the oldest global sidebar key override a newer generation', () => {
    window.localStorage.setItem('chrote.workspaceDock.v2', JSON.stringify({
      version: 2,
      workspaces: {
        terminal1: {
          openSidecars: [],
          sidecarPinned: false,
          sessionsWidth: 410,
          filesWidth: 360,
        },
      },
    }))
    window.localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: false }))

    expect(readSessionsDockState()).toEqual(DEFAULT_SESSIONS_DOCK_STATE)

    // Same for the generation before it, whose Files state is still read.
    window.localStorage.clear()
    window.localStorage.setItem('chrote.workspaceDock.v1', JSON.stringify({
      version: 1,
      workspaces: {
        terminal1: {
          filesCollapsed: false,
          filesWidth: 380,
        },
      },
    }))
    window.localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: false }))

    expect(readWorkspaceFilesDockState('terminal1')).toEqual({ open: true, pinned: true, width: 380 })
    expect(readSessionsDockState()).toEqual(DEFAULT_SESSIONS_DOCK_STATE)

    // Even a current generation the reader cannot make sense of outranks it:
    // unreadable is not the same as absent.
    window.localStorage.clear()
    window.localStorage.setItem('chrote.sessionsDock.v1', JSON.stringify({
      version: 1,
      state: 'malformed',
    }))
    window.localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: false }))

    expect(readSessionsDockState()).toEqual(DEFAULT_SESSIONS_DOCK_STATE)
  })

  it('migrates the legacy global sidebar collapse into the one Sessions presentation', () => {
    window.localStorage.setItem('chrote-dashboard-state', JSON.stringify({ sidebarCollapsed: false }))

    expect(readSessionsDockState()).toEqual({
      ...DEFAULT_SESSIONS_DOCK_STATE,
      open: true,
      pinned: true,
    })
  })

  it('keeps navigation, tree, and open-file state isolated per terminal workspace', () => {
    writeWorkspaceFilesState('terminal1', {
      ...DEFAULT_WORKSPACE_FILES_STATE,
      currentPath: '/srv/chrote',
      expandedPaths: ['/', '/srv', '/srv/chrote'],
      selectedPath: '/srv/chrote/README.md',
      treeScrollTop: 240,
      openPath: '/srv/chrote/README.md',
    })

    expect(readWorkspaceFilesState('terminal1')).toMatchObject({
      currentPath: '/srv/chrote',
      selectedPath: '/srv/chrote/README.md',
      treeScrollTop: 240,
      openPath: '/srv/chrote/README.md',
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
        openPath: 'relative/path',
      },
    }))

    expect(readWorkspaceFilesState('terminal1')).toEqual({
      ...DEFAULT_WORKSPACE_FILES_STATE,
      expandedPaths: ['/', '/srv'],
    })
  })
})
