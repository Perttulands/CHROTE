import { useCallback, useRef } from 'react'
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent } from 'react'
import type { FileItem } from './types'
import type { SavedPath } from './pinnedPaths'
import FileTree from '../FileTree'
import Menu from '../Menu'
import FileViewer, { getFileBadge, getPreviewKind, makeFileItemFromPath } from '../FileViewer'
import { DEFAULT_FILE_VIEW_STATE, type FileViewState } from '../workspaceFilesState'
import { FileContextMenu } from '../FileContextMenu'
import { formatDate, formatSize } from './utils'
import { savedPathLabel, type SavedPathGroup } from './savedPaths'
import type { useFilesView } from './useFilesView'
import { useResizableWidth } from '../../hooks/useResizableWidth'

type FilesViewContentProps = ReturnType<typeof useFilesView>

export default function FilesViewContent({
  uploadInputRef, pathInputRef, items, treeRefreshToken, setTreeRefreshToken,
  expandedPaths, setExpandedPaths, treeScrollTop, setTreeScrollTop, currentPath, history, historyIndex,
  loading, error, selectedPaths, sortBy, sortDir, viewMode, setViewMode,
  contentMode, setContentMode, searchQuery, setSearchQuery, contextMenu, setContextMenu,
  tabContextMenu, setTabContextMenu, renamingPath, renameValue, setRenameValue,
  createIntent, setCreateIntent, deleteTargets, setDeleteTargets,
  openFiles, activeFilePath, setActiveFilePath, fileViewStates, setFileViewStates,
  pinnedPaths, recentPaths, savedGroupsCollapsed, editingPath, setEditingPath,
  pathDraft, setPathDraft, setDraggingPaths, dropTargetPath, setDropTargetPath,
  operationLabel, currentPathPinned, explorerWidth, setExplorerWidth,
  loadDirectory, navigateTo, refreshCurrentPath, updateOpenFile,
  openFile, openSavedPath, closeOpenFile, closeAllOpenFiles, closeOtherOpenFiles,
  activeFile, selectedItems, visibleItems, toggleSort, goBack, goForward, goUp,
  handlePathSubmit, handleItemClick, handleContextMenu, beginRename, cancelRename,
  submitRename, startCreate, cancelCreate, confirmCreate, requestDelete, confirmDelete,
  handleUploadInput, handleDropUpload, downloadItems, copyPath, copySelectedPaths,
  copyRelativePath, openParentFolder, togglePin, toggleSavedGroup, saveActiveFile,
  handleInternalDragStart, handleFolderDragOver, handleFolderDrop, onSendPath,
  sendTargetLabel,
}: FilesViewContentProps) {
  const explorerRef = useRef<HTMLElement>(null)
  const widestExplorer = useCallback(() => 560, [])
  const explorerResize = useResizableWidth({
    elementRef: explorerRef,
    width: explorerWidth,
    minWidth: 180,
    maxWidth: widestExplorer,
    edge: 'right',
    onCommit: setExplorerWidth,
  })
  const resizedWorkbenchStyle = {
    '--fb-explorer-width': `${explorerResize.width}px`,
  } as CSSProperties

  const renderSavedPath = (item: SavedPath, className: string) => (
    <button
      key={`${item.kind}:${item.path}`}
      className={className}
      type="button"
      title={item.path}
      onClick={() => openSavedPath(item)}
      onContextMenu={(event) => {
        event.preventDefault()
        setContextMenu({
          x: event.clientX,
          y: event.clientY,
          item: item.kind === 'file' ? makeFileItemFromPath(item.path) : {
            name: savedPathLabel(item.path),
            path: item.path,
            isDir: true,
            size: 0,
            modified: new Date().toISOString(),
            type: '',
          },
        })
      }}
    >
      <span className="fb-mini-glyph">{item.kind === 'directory' ? 'DIR' : getFileBadge(makeFileItemFromPath(item.path))}</span>
      <span className="fb-saved-name">{savedPathLabel(item.path)}</span>
    </button>
  )

  const renderSavedPathGroup = (group: SavedPathGroup, title: string, paths: SavedPath[]) => {
    if (paths.length === 0) return null

    const collapsed = savedGroupsCollapsed[group]
    const listId = `fb-${group}-saved-list`

    return (
      <section className={`fb-sidebar-section fb-saved-section ${collapsed ? 'is-collapsed' : ''}`}>
        <button
          className="fb-section-title fb-section-toggle"
          type="button"
          aria-expanded={!collapsed}
          aria-controls={listId}
          onClick={() => toggleSavedGroup(group)}
        >
          <span className="fb-section-caret" aria-hidden="true">{collapsed ? '>' : 'v'}</span>
          <span>{title}</span>
          <span className="fb-section-count">{paths.length}</span>
        </button>
        {!collapsed && (
          <div id={listId} className="fb-saved-list">
            {paths.map(item => renderSavedPath(item, 'fb-saved-item'))}
          </div>
        )}
      </section>
    )
  }

  const renderCreateRow = () => {
    if (!createIntent || createIntent.parentPath !== currentPath) return null
    return (
      <div className="fb-row fb-row-editing">
        <span className="fb-file-glyph folder">{createIntent.kind === 'folder' ? 'DIR' : 'NEW'}</span>
        <input
          className="fb-rename-input"
          value={createIntent.name}
          onChange={(event) => setCreateIntent(prev => prev ? { ...prev, name: event.target.value } : prev)}
          onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
            if (event.key === 'Enter') void confirmCreate()
            if (event.key === 'Escape') cancelCreate()
          }}
          onBlur={() => void confirmCreate()}
          autoFocus
        />
      </div>
    )
  }

  const renderItemName = (item: FileItem) => {
    if (renamingPath === item.path) {
      return (
        <input
          className="fb-rename-input"
          value={renameValue}
          onChange={(event) => setRenameValue(event.target.value)}
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event: ReactKeyboardEvent<HTMLInputElement>) => {
            if (event.key === 'Enter') void submitRename(item)
            if (event.key === 'Escape') cancelRename()
          }}
          onBlur={() => void submitRename(item)}
          autoFocus
        />
      )
    }

    return <span className="fb-filename" title={item.name}>{item.name}</span>
  }

  const renderListItem = (item: FileItem) => {
    const selected = selectedPaths.has(item.path)
    const dropTarget = dropTargetPath === item.path

    return (
      <div
        className={`fb-row ${selected ? 'selected' : ''} ${dropTarget ? 'drop-target' : ''}`}
        key={item.path}
        role="row"
        tabIndex={0}
        draggable
        onClick={(event) => handleItemClick(item, event)}
        onDoubleClick={() => item.isDir ? navigateTo(item.path) : void openFile(item)}
        onContextMenu={(event) => handleContextMenu(event, item)}
        onDragStart={(event) => handleInternalDragStart(event, item)}
        onDragEnd={() => {
          setDraggingPaths([])
          setDropTargetPath(null)
        }}
        onDragOver={(event) => handleFolderDragOver(event, item)}
        onDragLeave={() => setDropTargetPath(null)}
        onDrop={(event) => void handleFolderDrop(event, item)}
      >
        <div className="fb-cell fb-cell-name">
          <span className={`fb-file-glyph ${item.isDir ? 'folder' : 'file'}`}>{getFileBadge(item)}</span>
          {renderItemName(item)}
        </div>
        <div className="fb-cell fb-cell-size">{item.isDir ? '-' : formatSize(item.size)}</div>
        <div className="fb-cell fb-cell-modified">{formatDate(item.modified)}</div>
      </div>
    )
  }

  const renderGridItem = (item: FileItem) => {
    const selected = selectedPaths.has(item.path)
    const dropTarget = dropTargetPath === item.path

    return (
      <div
        className={`fb-grid-item ${selected ? 'selected' : ''} ${dropTarget ? 'drop-target' : ''}`}
        key={item.path}
        tabIndex={0}
        draggable
        onClick={(event) => handleItemClick(item, event)}
        onDoubleClick={() => item.isDir ? navigateTo(item.path) : void openFile(item)}
        onContextMenu={(event) => handleContextMenu(event, item)}
        onDragStart={(event) => handleInternalDragStart(event, item)}
        onDragEnd={() => {
          setDraggingPaths([])
          setDropTargetPath(null)
        }}
        onDragOver={(event) => handleFolderDragOver(event, item)}
        onDragLeave={() => setDropTargetPath(null)}
        onDrop={(event) => void handleFolderDrop(event, item)}
      >
        <span className={`fb-grid-icon ${item.isDir ? 'folder' : 'file'}`}>{getFileBadge(item)}</span>
        <span className="fb-grid-name" title={item.name}>{item.name}</span>
        <span className="fb-grid-meta">{item.isDir ? 'Folder' : formatSize(item.size)}</span>
      </div>
    )
  }

  const activeFileItem: FileItem | null = activeFile ? {
    path: activeFile.path,
    name: activeFile.name,
    size: activeFile.size,
    type: activeFile.type,
    isDir: false,
    modified: '',
  } : null
  const activeViewState = activeFilePath ? fileViewStates[activeFilePath] || DEFAULT_FILE_VIEW_STATE : DEFAULT_FILE_VIEW_STATE
  const updateActiveViewState = (next: FileViewState) => {
    if (!activeFilePath) return
    setFileViewStates(previous => ({ ...previous, [activeFilePath]: next }))
  }
  const imageItems = items.filter(item => !item.isDir && getPreviewKind(item) === 'image')
  const activeImageIndex = activeFile ? imageItems.findIndex(item => item.path === activeFile.path) : -1
  const previousImage = activeImageIndex > 0 ? imageItems[activeImageIndex - 1] : null
  const nextImage = activeImageIndex >= 0 && activeImageIndex < imageItems.length - 1 ? imageItems[activeImageIndex + 1] : null

  const contextTargets = contextMenu?.item
    ? [contextMenu.item]
    : selectedItems

  return (
    <div className="fb-container files-view">
      <input
        ref={uploadInputRef}
        className="fb-hidden-input"
        type="file"
        multiple
        onChange={handleUploadInput}
      />

      <div className="fb-header">
        <div className="fb-header-left">
          <h2 className="fb-title">Files</h2>
          <div className="fb-tabs" aria-label="File workbench views">
            <button className="fb-tab active" type="button">Explorer</button>
            <button
              className={`fb-tab ${currentPathPinned ? 'active' : ''}`}
              type="button"
              title={currentPathPinned ? 'Unpin current folder' : 'Pin current folder'}
              aria-pressed={currentPathPinned}
              onClick={() => togglePin(currentPath, 'directory')}
            >
              {currentPathPinned ? 'Unpin' : 'Pin'}
            </button>
          </div>
        </div>
        <div className="fb-header-right">
          <button className="fb-btn" type="button" title="New File" onClick={() => startCreate('file')}>+ File</button>
          <button className="fb-btn" type="button" title="New Folder" onClick={() => startCreate('folder')}>+ Folder</button>
          <button className="fb-btn" type="button" title="Upload" onClick={() => uploadInputRef.current?.click()}>Upload</button>
          <button className="fb-btn" type="button" title="Refresh" disabled={loading} onClick={refreshCurrentPath}>Refresh</button>
        </div>
      </div>

      <div className="fb-workbench" style={resizedWorkbenchStyle}>
        <aside ref={explorerRef} className="fb-sidebar" aria-label="Explorer">
          <div className="fb-sidebar-header">
            <span>Explorer</span>
            <button className="fb-sidebar-action" type="button" title="Refresh tree" onClick={() => setTreeRefreshToken(previous => previous + 1)}>Refresh</button>
          </div>

          {renderSavedPathGroup('pinned', 'Pinned', pinnedPaths)}
          {renderSavedPathGroup('recent', 'Recent', recentPaths)}

          <section className="fb-sidebar-section fb-tree-section">
            <div className="fb-section-title">Workspace</div>
            <FileTree
              currentPath={currentPath}
              selectedPath={activeFilePath || currentPath}
              expandedPaths={expandedPaths}
              scrollTop={treeScrollTop}
              refreshToken={treeRefreshToken}
              onOpenDirectory={navigateTo}
              onOpenFile={item => void openFile(item)}
              onExpandedPathsChange={setExpandedPaths}
              onScrollTopChange={setTreeScrollTop}
              onItemContextMenu={(event, item) => handleContextMenu(event, item)}
            />
          </section>
        </aside>
        <div
          {...explorerResize.handleProps}
          className={`fb-explorer-resizer${explorerResize.resizing ? ' dragging' : ''}`}
          role="separator"
          aria-label="Resize Files explorer"
          aria-orientation="vertical"
          aria-valuenow={Math.round(explorerResize.width)}
          aria-valuemin={180}
          aria-valuemax={560}
          tabIndex={0}
        />

        <main className="fb-main">
          <div className="fb-toolbar">
            <div className="fb-toolbar-nav">
              <button className="fb-nav-btn" type="button" title="Back" disabled={historyIndex === 0} onClick={goBack}>Back</button>
              <button className="fb-nav-btn" type="button" title="Forward" disabled={historyIndex >= history.length - 1} onClick={goForward}>Forward</button>
              <button className="fb-nav-btn" type="button" title="Up" disabled={currentPath === '/'} onClick={goUp}>Up</button>
            </div>

            <div className="fb-pathbar">
              {editingPath ? (
                <input
                  ref={pathInputRef}
                  className="fb-path-input"
                  value={pathDraft}
                  onChange={(event) => setPathDraft(event.target.value)}
                  onBlur={handlePathSubmit}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') handlePathSubmit()
                    if (event.key === 'Escape') setEditingPath(false)
                  }}
                />
              ) : (
                <button className="fb-path-display" type="button" onClick={() => setEditingPath(true)} title="Edit path">
                  <span
                    className="fb-breadcrumb-root"
                    onClick={(event) => {
                      event.stopPropagation()
                      navigateTo('/')
                    }}
                  >
                    /
                  </span>
                  {currentPath.split('/').filter(Boolean).map((part, index, parts) => {
                    const path = `/${parts.slice(0, index + 1).join('/')}`
                    return (
                      <span className="fb-breadcrumb-segment" key={path}>
                        <span className="fb-breadcrumb-sep">/</span>
                        <span className="fb-breadcrumb-item">{part}</span>
                      </span>
                    )
                  })}
                </button>
              )}
            </div>

            <div className="fb-toolbar-actions">
              <button
                className={`fb-view-btn ${contentMode === 'folder' ? 'active' : ''}`}
                type="button"
                aria-pressed={contentMode === 'folder'}
                onClick={() => setContentMode('folder')}
              >
                Folder
              </button>
              <button
                className={`fb-view-btn ${contentMode === 'file' ? 'active' : ''}`}
                type="button"
                aria-pressed={contentMode === 'file'}
                disabled={!activeFile}
                onClick={() => setContentMode('file')}
              >
                File
              </button>
              {contentMode === 'folder' && (
                <>
                  <input
                    className="fb-search"
                    type="search"
                    placeholder="Filter"
                    value={searchQuery}
                    onChange={(event) => setSearchQuery(event.target.value)}
                  />
                  <button className={`fb-view-btn ${viewMode === 'list' ? 'active' : ''}`} type="button" title="List view" onClick={() => setViewMode('list')}>List</button>
                  <button className={`fb-view-btn ${viewMode === 'grid' ? 'active' : ''}`} type="button" title="Grid view" onClick={() => setViewMode('grid')}>Grid</button>
                </>
              )}
            </div>
          </div>

          <div
            className={`fb-content mode-${contentMode}`}
            onDragOver={(event) => event.preventDefault()}
            onDrop={handleDropUpload}
            onContextMenu={(event) => {
              if (!(event.target as HTMLElement).closest('.fb-row, .fb-grid-item')) {
                handleContextMenu(event, null)
              }
            }}
          >
            {contentMode === 'folder' && (
              <section className="fb-browser-pane">
              <div className="fb-list-container">
                {loading ? (
                  <div className="fb-loading">
                    <span className="fb-spinner" />
                    Loading...
                  </div>
                ) : error ? (
                  <div className="fb-error">
                    <span className="fb-error-icon">!</span>
                    <span className="fb-error-message">{error}</span>
                    <button className="fb-retry-btn" type="button" onClick={() => loadDirectory(currentPath)}>Retry</button>
                  </div>
                ) : viewMode === 'list' ? (
                  <div className="fb-list" role="grid">
                    <div className="fb-list-header" role="row">
                      <button className={`fb-column-header fb-cell-name ${sortBy === 'name' ? 'active' : ''}`} type="button" onClick={() => toggleSort('name')}>
                        Name {sortBy === 'name' ? (sortDir === 'asc' ? '^' : 'v') : ''}
                      </button>
                      <button className={`fb-column-header fb-cell-size ${sortBy === 'size' ? 'active' : ''}`} type="button" onClick={() => toggleSort('size')}>
                        Size {sortBy === 'size' ? (sortDir === 'asc' ? '^' : 'v') : ''}
                      </button>
                      <button className={`fb-column-header fb-cell-modified ${sortBy === 'modified' ? 'active' : ''}`} type="button" onClick={() => toggleSort('modified')}>
                        Modified {sortBy === 'modified' ? (sortDir === 'asc' ? '^' : 'v') : ''}
                      </button>
                    </div>
                    <div className="fb-list-body">
                      {renderCreateRow()}
                      {visibleItems.length === 0 ? (
                        <div className="fb-empty">
                          <span className="fb-empty-icon">EMPTY</span>
                          {searchQuery ? 'No matching files' : 'This folder is empty'}
                        </div>
                      ) : (
                        visibleItems.map(renderListItem)
                      )}
                    </div>
                  </div>
                ) : (
                  <div className="fb-grid" role="grid">
                    {renderCreateRow()}
                    {visibleItems.length === 0 ? (
                      <div className="fb-empty">
                        <span className="fb-empty-icon">EMPTY</span>
                        {searchQuery ? 'No matching files' : 'This folder is empty'}
                      </div>
                    ) : (
                      visibleItems.map(renderGridItem)
                    )}
                  </div>
                )}
              </div>

              <div className="fb-statusbar">
                <span>{visibleItems.length} items</span>
                {selectedPaths.size > 0 && <span>{selectedPaths.size} selected</span>}
                {operationLabel && <span className="fb-statusbar-operation">{operationLabel}...</span>}
              </div>
            </section>
            )}

            {contentMode === 'file' && (
              <aside className="fb-editor-pane">
              <div className="fb-editor-tabs">
                {openFiles.length === 0 ? (
                  <span className="fb-editor-placeholder-tab">Preview</span>
                ) : (
                  <>
                    {openFiles.map(file => (
                      <button
                        key={file.path}
                        className={`fb-editor-tab ${file.path === activeFilePath ? 'active' : ''}`}
                        type="button"
                        title={file.path}
                        onClick={() => {
                          setActiveFilePath(file.path)
                          setContentMode('file')
                        }}
                        onContextMenu={(event) => {
                          event.preventDefault()
                          event.stopPropagation()
                          setTabContextMenu({ x: event.clientX, y: event.clientY, path: file.path })
                        }}
                      >
                        <span>{file.dirty ? '* ' : ''}{file.name}</span>
                        <span
                          className="fb-editor-tab-close"
                          role="button"
                          tabIndex={0}
                          onClick={(event) => {
                            event.stopPropagation()
                            closeOpenFile(file.path)
                          }}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter' || event.key === ' ') {
                              event.stopPropagation()
                              closeOpenFile(file.path)
                            }
                          }}
                        >
                          x
                        </span>
                      </button>
                    ))}
                    {openFiles.length > 1 && (
                      <button
                        className="fb-editor-tabs-close-all"
                        type="button"
                        title="Close all open files"
                        onClick={closeAllOpenFiles}
                      >
                        Close All
                      </button>
                    )}
                  </>
                )}
              </div>

              {activeFile ? (
                <div className="fb-editor">
                  <div className="fb-editor-header">
                    <div className="fb-editor-title">
                      <span className="fb-file-glyph file">{getFileBadge(makeFileItemFromPath(activeFile.path))}</span>
                      <div>
                        <strong>{activeFile.name}</strong>
                        <span>{activeFile.path}</span>
                      </div>
                    </div>
                    <div className="fb-editor-actions">
                      {activeFile.kind === 'text' && (
                        <button className="fb-btn" type="button" disabled={!activeFile.dirty || activeFile.loading} onClick={() => void saveActiveFile()}>Save</button>
                      )}
                      <button className="fb-btn" type="button" onClick={() => downloadItems([makeFileItemFromPath(activeFile.path)])}>Download</button>
                      <button className="fb-btn" type="button" onClick={() => copyPath(activeFile.path)}>Copy Path</button>
                      <button
                        className="fb-btn"
                        type="button"
                        disabled={!onSendPath}
                        title={onSendPath ? `Send path to ${sendTargetLabel || 'focused session'}` : 'Focus a terminal session first'}
                        onClick={() => onSendPath?.(activeFile.path)}
                      >
                        Send Path
                      </button>
                    </div>
                  </div>

                  {activeFile.loading ? (
                    <div className="fb-editor-empty">Loading file...</div>
                  ) : activeFile.error ? (
                    <div className="fb-editor-empty">{activeFile.error}</div>
                  ) : activeFileItem ? (
                    <FileViewer
                      item={activeFileItem}
                      content={activeFile.kind === 'text' ? activeFile.content : undefined}
                      editable={activeFile.kind === 'text'}
                      onContentChange={content => updateOpenFile(activeFile.path, { content, dirty: true })}
                      viewState={activeViewState}
                      onViewStateChange={updateActiveViewState}
                      onPrevious={previousImage ? () => void openFile(previousImage) : undefined}
                      onNext={nextImage ? () => void openFile(nextImage) : undefined}
                    />
                  ) : null}
                </div>
              ) : (
                <div className="fb-editor-empty">
                  <strong>No file selected</strong>
                  <span>Select a file to preview or edit it here.</span>
                </div>
              )}
              </aside>
            )}
          </div>
        </main>
      </div>

      {tabContextMenu && openFiles.length > 1 && (
        <Menu
          at={{ x: tabContextMenu.x, y: tabContextMenu.y }}
          label="Open file actions"
          zIndex={2200}
          estimatedSize={{ width: 200, height: 70 }}
          onClose={() => setTabContextMenu(null)}
          groups={[{
            id: 'tabs',
            rows: [
              { id: 'close-others', label: 'Close others', onSelect: () => closeOtherOpenFiles(tabContextMenu.path) },
              { id: 'close-all', label: 'Close all', onSelect: closeAllOpenFiles },
            ],
          }]}
        />
      )}

      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          item={contextMenu.item}
          itemPinned={Boolean(contextMenu.item && pinnedPaths.some(item => item.path === contextMenu.item!.path))}
          currentPathPinned={currentPathPinned}
          onClose={() => setContextMenu(null)}
          onOpen={item => item.isDir ? navigateTo(item.path) : void openFile(item)}
          onDownload={item => downloadItems([item])}
          onRename={beginRename}
          onTogglePin={item => togglePin(item.path, item.isDir ? 'directory' : 'file')}
          onCopyPath={copyPath}
          onCopySelectedPaths={contextMenu.item || selectedItems.length > 0 ? () => copySelectedPaths(contextTargets) : undefined}
          onCopyRelativePath={copyRelativePath}
          onOpenParent={openParentFolder}
          onDelete={() => requestDelete(contextTargets)}
          onNewFile={() => startCreate('file')}
          onNewFolder={() => startCreate('folder')}
          onUpload={() => uploadInputRef.current?.click()}
          onRefresh={refreshCurrentPath}
          onCopyCurrentPath={() => copyPath(currentPath)}
          onToggleCurrentPathPin={() => togglePin(currentPath, 'directory')}
        />
      )}

      {deleteTargets && (
        <div className="fb-dialog-overlay">
          <div className="fb-dialog fb-dialog-danger">
            <div className="fb-dialog-header">
              <h3>Delete {deleteTargets.length === 1 ? deleteTargets[0].name : `${deleteTargets.length} items`}</h3>
              <button className="fb-dialog-close" type="button" onClick={() => setDeleteTargets(null)}>x</button>
            </div>
            <div className="fb-dialog-body">
              <p className="fb-dialog-message">
                This permanently removes the selected {deleteTargets.length === 1 ? 'item' : 'items'} from disk.
              </p>
            </div>
            <div className="fb-dialog-footer">
              <button className="fb-dialog-btn fb-dialog-btn-cancel" type="button" onClick={() => setDeleteTargets(null)}>Cancel</button>
              <button className="fb-dialog-btn fb-dialog-btn-danger" type="button" onClick={() => void confirmDelete()}>Delete</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
