import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import {
  FileItem,
  FileBrowserState,
  OperationState,
  ViewTab,
  ContextMenuState,
  toDisplayPath,
  getRootDisplayName,
} from './types'
import {
  fetchDirectory,
  createFolder,
  renameItem,
  deleteItem,
  uploadFiles,
  pathExists,
  getDownloadUrl,
  getErrorMessage,
} from './fileService'
import { ErrorToast } from './components/ErrorToast'

// ============================================
// UTILITY FUNCTIONS
// ============================================

function formatSize(bytes: number): string {
  if (bytes === 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffDays === 0) {
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
    if (diffHours === 0) {
      const diffMins = Math.floor(diffMs / (1000 * 60))
      return diffMins <= 1 ? 'Just now' : `${diffMins} min ago`
    }
    return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`
  }

  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays} days ago`

  return date.toLocaleDateString()
}

function getFileIcon(item: FileItem): string {
  if (item.isDir) return '📁'

  const ext = item.name.split('.').pop()?.toLowerCase() || ''
  const iconMap: Record<string, string> = {
    // Code
    js: '📜', ts: '📜', jsx: '📜', tsx: '📜',
    py: '🐍', rb: '💎', go: '🔵', rs: '🦀',
    java: '☕', c: '⚙️', cpp: '⚙️', h: '⚙️',
    cs: '🔷', php: '🐘', swift: '🍎',
    // Web
    html: '🌐', css: '🎨', scss: '🎨', less: '🎨',
    // Data
    json: '📋', yaml: '📋', yml: '📋', xml: '📋',
    csv: '📊', sql: '🗄️',
    // Documents
    md: '📝', txt: '📄', pdf: '📕', doc: '📘', docx: '📘',
    xls: '📗', xlsx: '📗', ppt: '📙', pptx: '📙',
    // Media
    png: '🖼️', jpg: '🖼️', jpeg: '🖼️', gif: '🖼️', svg: '🖼️', webp: '🖼️',
    mp3: '🎵', wav: '🎵', flac: '🎵', ogg: '🎵',
    mp4: '🎬', mkv: '🎬', avi: '🎬', mov: '🎬', webm: '🎬',
    // Archives
    zip: '📦', tar: '📦', gz: '📦', rar: '📦', '7z': '📦',
    // Config
    env: '🔐', gitignore: '🚫', dockerfile: '🐳',
    // Shell
    sh: '💻', bash: '💻', zsh: '💻', fish: '💻',
  }

  return iconMap[ext] || '📄'
}

// ============================================
// SUB-COMPONENTS
// ============================================

// Breadcrumb Navigation with Windows path display
function Breadcrumbs({
  path,
  onNavigate
}: {
  path: string
  onNavigate: (path: string) => void
}) {
  const parts = path.split('/').filter(Boolean)
  const isRoot = parts.length === 0

  // Display Windows-style paths
  const displayPath = toDisplayPath(path)

  return (
    <nav className="fb-breadcrumbs">
      <button
        className="fb-breadcrumb-item fb-breadcrumb-root"
        onClick={() => onNavigate('/')}
        title="Root"
      >
        {isRoot ? displayPath || '/' : '/'}
      </button>
      {parts.map((part, index) => {
        const partPath = '/' + parts.slice(0, index + 1).join('/')
        const isLast = index === parts.length - 1
        // At root level, show Windows paths
        const displayName = index === 0 ? getRootDisplayName(part) : part

        return (
          <span key={partPath} className="fb-breadcrumb-segment">
            <span className="fb-breadcrumb-sep">/</span>
            <button
              className={`fb-breadcrumb-item ${isLast ? 'active' : ''}`}
              onClick={() => onNavigate(partPath)}
              disabled={isLast}
            >
              {displayName}
            </button>
          </span>
        )
      })}
    </nav>
  )
}

// File/Folder Icon Component
function FileIcon({ item }: { item: FileItem }) {
  return (
    <span className={`fb-icon ${item.isDir ? 'fb-icon-folder' : 'fb-icon-file'}`}>
      {getFileIcon(item)}
    </span>
  )
}

// Column Header (sortable)
function ColumnHeader({
  label,
  sortKey,
  currentSort,
  currentDir,
  onSort,
  className,
}: {
  label: string
  sortKey: 'name' | 'size' | 'modified'
  currentSort: string
  currentDir: string
  onSort: (key: 'name' | 'size' | 'modified') => void
  className?: string
}) {
  const isActive = currentSort === sortKey

  return (
    <button
      className={`fb-column-header ${className || ''} ${isActive ? 'active' : ''}`}
      onClick={() => onSort(sortKey)}
    >
      {label}
      {isActive && (
        <span className="fb-sort-indicator">
          {currentDir === 'asc' ? '▲' : '▼'}
        </span>
      )}
    </button>
  )
}

// File Row Component
function FileRow({
  item,
  isSelected,
  onSelect,
  onNavigate,
  onContextMenu,
  onRename,
  isRenaming,
  renameValue,
  setRenameValue,
  onRenameSubmit,
  onRenameCancel,
  isAtRoot,
  disabled,
}: {
  item: FileItem
  isSelected: boolean
  onSelect: (e: React.MouseEvent) => void
  onNavigate: () => void
  onContextMenu: (e: React.MouseEvent) => void
  onRename: () => void
  isRenaming: boolean
  renameValue: string
  setRenameValue: (value: string) => void
  onRenameSubmit: () => void
  onRenameCancel: () => void
  isAtRoot: boolean
  disabled?: boolean
}) {
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (isRenaming && inputRef.current) {
      inputRef.current.focus()
      const ext = item.name.includes('.') ? item.name.lastIndexOf('.') : item.name.length
      inputRef.current.setSelectionRange(0, ext)
    }
  }, [isRenaming, item.name])

  const handleDoubleClick = () => {
    if (disabled) return
    if (item.isDir) {
      onNavigate()
    } else {
      window.open(getDownloadUrl(item.path), '_blank')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (disabled) return
    if (e.key === 'Enter' && !isRenaming) {
      handleDoubleClick()
    } else if (e.key === 'F2' && !isRenaming) {
      onRename()
    }
  }

  const handleRenameKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      onRenameSubmit()
    } else if (e.key === 'Escape') {
      onRenameCancel()
    }
  }

  // At root level, show Windows paths like E:/Code
  const displayName = isAtRoot ? getRootDisplayName(item.name) : item.name

  return (
    <div
      className={`fb-row ${isSelected ? 'selected' : ''} ${disabled ? 'disabled' : ''}`}
      onClick={disabled ? undefined : onSelect}
      onDoubleClick={handleDoubleClick}
      onContextMenu={disabled ? undefined : onContextMenu}
      onKeyDown={handleKeyDown}
      tabIndex={disabled ? -1 : 0}
      role="row"
    >
      <div className="fb-cell fb-cell-name">
        <FileIcon item={item} />
        {isRenaming ? (
          <input
            ref={inputRef}
            type="text"
            className="fb-rename-input"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            onKeyDown={handleRenameKeyDown}
            onBlur={onRenameCancel}
          />
        ) : (
          <span className="fb-filename">{displayName}</span>
        )}
      </div>
      <div className="fb-cell fb-cell-size">
        {item.isDir ? '-' : formatSize(item.size)}
      </div>
      <div className="fb-cell fb-cell-modified">
        {formatDate(item.modified)}
      </div>
    </div>
  )
}

// Grid Item Component
function FileGridItem({
  item,
  isSelected,
  onSelect,
  onNavigate,
  onContextMenu,
  isAtRoot,
  disabled,
}: {
  item: FileItem
  isSelected: boolean
  onSelect: (e: React.MouseEvent) => void
  onNavigate: () => void
  onContextMenu: (e: React.MouseEvent) => void
  isAtRoot: boolean
  disabled?: boolean
}) {
  const handleDoubleClick = () => {
    if (disabled) return
    if (item.isDir) {
      onNavigate()
    } else {
      window.open(getDownloadUrl(item.path), '_blank')
    }
  }

  const displayName = isAtRoot ? getRootDisplayName(item.name) : item.name

  return (
    <div
      className={`fb-grid-item ${isSelected ? 'selected' : ''} ${disabled ? 'disabled' : ''}`}
      onClick={disabled ? undefined : onSelect}
      onDoubleClick={handleDoubleClick}
      onContextMenu={disabled ? undefined : onContextMenu}
      tabIndex={disabled ? -1 : 0}
      role="gridcell"
    >
      <div className="fb-grid-icon">
        {getFileIcon(item)}
      </div>
      <div className="fb-grid-name" title={displayName}>
        {displayName}
      </div>
    </div>
  )
}

// Context Menu
function ContextMenu({
  x,
  y,
  item,
  onClose,
  onDownload,
  onRename,
  onDelete,
  onCopyPath,
  onNewFolder,
}: {
  x: number
  y: number
  item: FileItem | null
  onClose: () => void
  onDownload: () => void
  onRename: () => void
  onDelete: () => void
  onCopyPath: () => void
  onNewFolder: () => void
}) {
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose()
      }
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [onClose])

  const style: React.CSSProperties = {
    position: 'fixed',
    top: y,
    left: x,
    zIndex: 1000,
  }

  return (
    <div ref={menuRef} className="fb-context-menu" style={style}>
      {item && !item.isDir && (
        <button className="fb-context-item" onClick={onDownload}>
          <span className="fb-context-icon">⬇</span>
          Download
        </button>
      )}
      {item && (
        <>
          <button className="fb-context-item" onClick={onRename}>
            <span className="fb-context-icon">✏</span>
            Rename
          </button>
          <button className="fb-context-item" onClick={onCopyPath}>
            <span className="fb-context-icon">📋</span>
            Copy Path
          </button>
          <div className="fb-context-divider" />
          <button className="fb-context-item fb-context-danger" onClick={onDelete}>
            <span className="fb-context-icon">🗑</span>
            Delete
          </button>
        </>
      )}
      {!item && (
        <button className="fb-context-item" onClick={onNewFolder}>
          <span className="fb-context-icon">📁</span>
          New Folder
        </button>
      )}
    </div>
  )
}

// New Folder Dialog
function NewFolderDialog({
  onClose,
  onCreate,
  loading,
  error,
}: {
  onClose: () => void
  onCreate: (name: string) => void
  loading?: boolean
  error?: string | null
}) {
  const [name, setName] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (name.trim() && !loading) {
      onCreate(name.trim())
    }
  }

  return (
    <div className="fb-dialog-overlay" onClick={onClose}>
      <div className="fb-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="fb-dialog-header">
          <h3>New Folder</h3>
          <button className="fb-dialog-close" onClick={onClose} disabled={loading}>×</button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="fb-dialog-body">
            <label className="fb-dialog-label">Folder name</label>
            <input
              ref={inputRef}
              type="text"
              className="fb-dialog-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="New folder"
              disabled={loading}
            />
            {error && (
              <div className="fb-dialog-error">{error}</div>
            )}
          </div>
          <div className="fb-dialog-footer">
            <button
              type="button"
              className="fb-dialog-btn fb-dialog-btn-cancel"
              onClick={onClose}
              disabled={loading}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="fb-dialog-btn fb-dialog-btn-primary"
              disabled={!name.trim() || loading}
            >
              {loading ? 'Creating...' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// Delete Confirmation Dialog
function DeleteDialog({
  item,
  onClose,
  onConfirm,
  loading,
  error,
}: {
  item: FileItem
  onClose: () => void
  onConfirm: () => void
  loading?: boolean
  error?: string | null
}) {
  return (
    <div className="fb-dialog-overlay" onClick={onClose}>
      <div className="fb-dialog fb-dialog-danger" onClick={(e) => e.stopPropagation()}>
        <div className="fb-dialog-header">
          <h3>Delete {item.isDir ? 'Folder' : 'File'}</h3>
          <button className="fb-dialog-close" onClick={onClose} disabled={loading}>×</button>
        </div>
        <div className="fb-dialog-body">
          <p className="fb-dialog-message">
            Are you sure you want to delete <strong>{item.name}</strong>?
            {item.isDir && ' This will delete all contents inside.'}
          </p>
          {error && (
            <div className="fb-dialog-error">{error}</div>
          )}
        </div>
        <div className="fb-dialog-footer">
          <button
            className="fb-dialog-btn fb-dialog-btn-cancel"
            onClick={onClose}
            disabled={loading}
          >
            Cancel
          </button>
          <button
            className="fb-dialog-btn fb-dialog-btn-danger"
            onClick={onConfirm}
            disabled={loading}
          >
            {loading ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      </div>
    </div>
  )
}

// Upload Zone with proper error display
function UploadZone({
  currentPath,
  onUploadComplete,
  onError,
}: {
  currentPath: string
  onUploadComplete: () => void
  onError: (message: string) => void
}) {
  const [isDragging, setIsDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(true)
  }

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
  }

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)

    const files = e.dataTransfer.files
    if (files.length > 0) {
      await handleUpload(files)
    }
  }

  const handleUpload = async (files: FileList) => {
    setUploading(true)
    setUploadProgress(`Uploading ${files.length} file${files.length > 1 ? 's' : ''}...`)

    try {
      await uploadFiles(currentPath, files)
      setUploadProgress('Upload complete!')
      setTimeout(() => {
        setUploadProgress(null)
        onUploadComplete()
      }, 1500)
    } catch (error) {
      const message = getErrorMessage(error, 'upload')
      setUploadProgress(null)
      onError(message)
    } finally {
      setUploading(false)
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      handleUpload(e.target.files)
    }
  }

  return (
    <div
      className={`fb-upload-zone ${isDragging ? 'dragging' : ''} ${uploading ? 'uploading' : ''}`}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <input
        ref={inputRef}
        type="file"
        multiple
        onChange={handleFileSelect}
        style={{ display: 'none' }}
      />
      {uploadProgress ? (
        <span className="fb-upload-status">{uploadProgress}</span>
      ) : (
        <button
          className="fb-upload-btn"
          onClick={() => inputRef.current?.click()}
          disabled={uploading}
        >
          <span className="fb-upload-icon">⬆</span>
          Upload
        </button>
      )}
    </div>
  )
}

// Inbox Panel - Drop zone for sending files with proper error handling
function InboxPanel({ onError }: { onError: (message: string) => void }) {
  const [files, setFiles] = useState<File[]>([])
  const [note, setNote] = useState('')
  const [isDragging, setIsDragging] = useState(false)
  const [sending, setSending] = useState(false)
  const [status, setStatus] = useState<'idle' | 'success' | 'error'>('idle')
  const inputRef = useRef<HTMLInputElement>(null)

  // Use /code/incoming (container path)
  const INBOX_PATH = '/code/incoming'

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(true)
  }

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
    if (e.dataTransfer.files.length > 0) {
      setFiles(Array.from(e.dataTransfer.files))
      setStatus('idle')
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      setFiles(Array.from(e.target.files))
      setStatus('idle')
    }
  }

  const removeFile = (index: number) => {
    setFiles(prev => prev.filter((_, i) => i !== index))
  }

  const handleSend = async () => {
    if (files.length === 0) return
    setSending(true)

    try {
      // Ensure incoming folder exists
      const exists = await pathExists(INBOX_PATH)
      if (!exists) {
        await createFolder('/code', 'incoming')
      }

      // Upload all files
      await uploadFiles(INBOX_PATH, files)

      // Upload note file if provided
      if (note.trim() && files.length > 0) {
        const noteFile = new File([note.trim()], `${files[0].name}.note`, { type: 'text/plain' })
        await uploadFiles(INBOX_PATH, [noteFile])
      }

      setStatus('success')
      setFiles([])
      setNote('')
      if (inputRef.current) inputRef.current.value = ''
      setTimeout(() => setStatus('idle'), 2000)
    } catch (error) {
      setStatus('error')
      const message = getErrorMessage(error, 'upload')
      onError(`Failed to send files: ${message}`)
      setTimeout(() => setStatus('idle'), 2000)
    } finally {
      setSending(false)
    }
  }

  const totalSize = files.reduce((sum, f) => sum + f.size, 0)

  return (
    <div className={`inbox-panel ${isDragging ? 'dragging' : ''} ${status} ${files.length > 0 ? 'has-files' : ''}`}>
      <div
        className="inbox-dropzone"
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => files.length === 0 && inputRef.current?.click()}
      >
        <input
          ref={inputRef}
          type="file"
          multiple
          onChange={handleFileSelect}
          style={{ display: 'none' }}
        />
        {files.length > 0 ? (
          <div className="inbox-files-list">
            {files.map((file, index) => (
              <div key={`${file.name}-${index}`} className="inbox-file-item">
                <span className="inbox-file-icon">📄</span>
                <span className="inbox-file-name">{file.name}</span>
                <span className="inbox-file-size">{formatSize(file.size)}</span>
                <button
                  className="inbox-file-remove"
                  onClick={(e) => { e.stopPropagation(); removeFile(index) }}
                >
                  ×
                </button>
              </div>
            ))}
            <button
              className="inbox-add-more"
              onClick={(e) => { e.stopPropagation(); inputRef.current?.click() }}
            >
              + Add more files
            </button>
          </div>
        ) : (
          <div className="inbox-placeholder">
            <span className="inbox-icon">📬</span>
            <span className="inbox-title">Send a package to E:/Code/incoming</span>
            <span className="inbox-subtitle">Drop files here or click to browse</span>
          </div>
        )}
      </div>

      <div className="inbox-bottom">
        <textarea
          className="inbox-note"
          placeholder="Add a note for the agent..."
          value={note}
          onChange={(e) => setNote(e.target.value)}
          disabled={sending}
          rows={2}
        />
        <div className="inbox-actions">
          {files.length > 0 && (
            <span className="inbox-summary">
              {files.length} file{files.length > 1 ? 's' : ''} · {formatSize(totalSize)}
            </span>
          )}
          <button
            className="inbox-send"
            onClick={handleSend}
            disabled={files.length === 0 || sending}
          >
            {sending ? 'Sending...' : status === 'success' ? '✓ Sent!' : `Send ${files.length > 0 ? `(${files.length})` : ''}`}
          </button>
        </div>
      </div>
    </div>
  )
}

// Info Panel with correct Windows paths
function InfoPanel() {
  return (
    <div className="fb-info-panel">
      <div className="fb-info-card">
        <h3>Mounted Volumes</h3>
        <div className="fb-info-item">
          <span className="fb-info-label">/code</span>
          <span className="fb-info-value">E:/Code</span>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">/vault</span>
          <span className="fb-info-value">E:/Vault</span>
        </div>
      </div>

      <div className="fb-info-card">
        <h3>Keyboard Shortcuts</h3>
        <div className="fb-info-item">
          <span className="fb-info-label">Open/Enter</span>
          <kbd className="fb-kbd">Enter</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Go Back</span>
          <kbd className="fb-kbd">Backspace</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Rename</span>
          <kbd className="fb-kbd">F2</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Delete</span>
          <kbd className="fb-kbd">Del</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Select All</span>
          <kbd className="fb-kbd">Ctrl+A</kbd>
        </div>
        <div className="fb-info-item">
          <span className="fb-info-label">Refresh</span>
          <kbd className="fb-kbd">F5</kbd>
        </div>
      </div>

      <div className="fb-info-card">
        <h3>Tips</h3>
        <ul className="fb-info-tips">
          <li>Double-click folders to open them</li>
          <li>Double-click files to download</li>
          <li>Right-click for more options</li>
          <li>Drag and drop files to upload</li>
        </ul>
      </div>
    </div>
  )
}

// ============================================
// MAIN COMPONENT
// ============================================

function FilesView() {
  const [activeTab, setActiveTab] = useState<ViewTab>('browser')
  const [state, setState] = useState<FileBrowserState>({
    items: [],
    loading: true,
    error: null,
    currentPath: '/',
    selectedItems: new Set(),
    sortBy: 'name',
    sortDir: 'asc',
    viewMode: 'list',
    searchQuery: '',
  })
  const [operation, setOperation] = useState<OperationState>({
    type: null,
    loading: false,
    error: null,
  })
  const [toast, setToast] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [renamingItem, setRenamingItem] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [showNewFolderDialog, setShowNewFolderDialog] = useState(false)
  const [deleteItemState, setDeleteItemState] = useState<FileItem | null>(null)
  const [navigationHistory, setNavigationHistory] = useState<string[]>(['/'])
  const [historyIndex, setHistoryIndex] = useState(0)

  // Show error toast
  const showError = useCallback((message: string) => {
    setToast(message)
  }, [])

  // Clear operation state
  const clearOperation = useCallback(() => {
    setOperation({ type: null, loading: false, error: null })
  }, [])

  // Load directory contents - NO silent fallback
  const loadDirectory = useCallback(async (path: string) => {
    setState(prev => ({ ...prev, loading: true, error: null }))

    try {
      const items = await fetchDirectory(path)
      setState(prev => ({
        ...prev,
        items,
        loading: false,
        currentPath: path,
        selectedItems: new Set(),
        error: null,
      }))
    } catch (error) {
      const message = getErrorMessage(error, 'fetch')
      setState(prev => ({
        ...prev,
        loading: false,
        error: message,
        items: [], // Clear items on error, don't silently keep old data
      }))
    }
  }, [])

  // Initial load
  useEffect(() => {
    loadDirectory('/')
  }, [loadDirectory])

  // Navigate to path
  const navigateTo = useCallback((path: string, addToHistory = true) => {
    if (addToHistory) {
      const newHistory = [...navigationHistory.slice(0, historyIndex + 1), path]
      setNavigationHistory(newHistory)
      setHistoryIndex(newHistory.length - 1)
    }
    loadDirectory(path)
  }, [loadDirectory, navigationHistory, historyIndex])

  // Go back
  const goBack = useCallback(() => {
    if (historyIndex > 0) {
      const newIndex = historyIndex - 1
      setHistoryIndex(newIndex)
      loadDirectory(navigationHistory[newIndex])
    }
  }, [historyIndex, navigationHistory, loadDirectory])

  // Go forward
  const goForward = useCallback(() => {
    if (historyIndex < navigationHistory.length - 1) {
      const newIndex = historyIndex + 1
      setHistoryIndex(newIndex)
      loadDirectory(navigationHistory[newIndex])
    }
  }, [historyIndex, navigationHistory, loadDirectory])

  // Go to parent directory
  const goUp = useCallback(() => {
    const parts = state.currentPath.split('/').filter(Boolean)
    if (parts.length > 0) {
      parts.pop()
      navigateTo('/' + parts.join('/'))
    }
  }, [state.currentPath, navigateTo])

  // Sort items
  const sortedItems = useMemo(() => {
    const items = [...state.items]

    const filtered = state.searchQuery
      ? items.filter(item => item.name.toLowerCase().includes(state.searchQuery.toLowerCase()))
      : items

    filtered.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1

      let comparison = 0
      switch (state.sortBy) {
        case 'name':
          comparison = a.name.localeCompare(b.name)
          break
        case 'size':
          comparison = a.size - b.size
          break
        case 'modified':
          comparison = new Date(a.modified).getTime() - new Date(b.modified).getTime()
          break
      }

      return state.sortDir === 'asc' ? comparison : -comparison
    })

    return filtered
  }, [state.items, state.sortBy, state.sortDir, state.searchQuery])

  // Handle sort
  const handleSort = (key: 'name' | 'size' | 'modified') => {
    setState(prev => ({
      ...prev,
      sortBy: key,
      sortDir: prev.sortBy === key && prev.sortDir === 'asc' ? 'desc' : 'asc',
    }))
  }

  // Handle selection
  const handleSelect = (item: FileItem, e: React.MouseEvent) => {
    e.preventDefault()

    if (e.ctrlKey || e.metaKey) {
      setState(prev => {
        const newSelected = new Set(prev.selectedItems)
        if (newSelected.has(item.name)) {
          newSelected.delete(item.name)
        } else {
          newSelected.add(item.name)
        }
        return { ...prev, selectedItems: newSelected }
      })
    } else if (e.shiftKey && state.selectedItems.size > 0) {
      const lastSelected = Array.from(state.selectedItems).pop()
      const lastIndex = sortedItems.findIndex(i => i.name === lastSelected)
      const currentIndex = sortedItems.findIndex(i => i.name === item.name)
      const start = Math.min(lastIndex, currentIndex)
      const end = Math.max(lastIndex, currentIndex)

      const rangeSelection = new Set(
        sortedItems.slice(start, end + 1).map(i => i.name)
      )
      setState(prev => ({ ...prev, selectedItems: rangeSelection }))
    } else {
      setState(prev => ({ ...prev, selectedItems: new Set([item.name]) }))
    }
  }

  // Handle context menu
  const handleContextMenu = (item: FileItem | null, e: React.MouseEvent) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, item })
    if (item && !state.selectedItems.has(item.name)) {
      setState(prev => ({ ...prev, selectedItems: new Set([item.name]) }))
    }
  }

  // Handle rename - with proper error state
  const startRename = (item: FileItem) => {
    setRenamingItem(item.name)
    setRenameValue(item.name)
    setContextMenu(null)
  }

  const submitRename = async () => {
    if (!renamingItem || !renameValue.trim() || renameValue === renamingItem) {
      cancelRename()
      return
    }

    const item = state.items.find(i => i.name === renamingItem)
    if (!item) return

    const newPath = state.currentPath === '/'
      ? `/${renameValue}`
      : `${state.currentPath}/${renameValue}`

    setOperation({ type: 'rename', loading: true, error: null, target: item.name })

    try {
      await renameItem(item.path, newPath)
      await loadDirectory(state.currentPath)
      clearOperation()
    } catch (error) {
      const message = getErrorMessage(error, 'rename')
      setOperation({ type: 'rename', loading: false, error: message, target: item.name })
      showError(message)
    } finally {
      cancelRename()
    }
  }

  const cancelRename = () => {
    setRenamingItem(null)
    setRenameValue('')
  }

  // Handle delete - with proper error state
  const handleDelete = async () => {
    if (!deleteItemState) return

    setOperation({ type: 'delete', loading: true, error: null, target: deleteItemState.name })

    try {
      await deleteItem(deleteItemState.path)
      await loadDirectory(state.currentPath)
      clearOperation()
      setDeleteItemState(null)
    } catch (error) {
      const message = getErrorMessage(error, 'delete')
      setOperation({ type: 'delete', loading: false, error: message, target: deleteItemState.name })
      showError(message)
    }
  }

  // Handle new folder - with proper error state
  const handleNewFolder = async (name: string) => {
    setOperation({ type: 'create', loading: true, error: null, target: name })

    try {
      await createFolder(state.currentPath, name)
      await loadDirectory(state.currentPath)
      clearOperation()
      setShowNewFolderDialog(false)
    } catch (error) {
      const message = getErrorMessage(error, 'create')
      setOperation({ type: 'create', loading: false, error: message, target: name })
      // Don't close dialog, show error in dialog
    }
  }

  // Handle download
  const handleDownload = () => {
    if (contextMenu?.item && !contextMenu.item.isDir) {
      window.open(getDownloadUrl(contextMenu.item.path), '_blank')
    }
    setContextMenu(null)
  }

  // Handle copy path - copy Windows path
  const handleCopyPath = () => {
    if (contextMenu?.item) {
      const displayPath = toDisplayPath(contextMenu.item.path)
      navigator.clipboard.writeText(displayPath)
    }
    setContextMenu(null)
  }

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (activeTab !== 'browser') return
      if ((e.target as HTMLElement).tagName === 'INPUT' || (e.target as HTMLElement).tagName === 'TEXTAREA') return

      if (e.key === 'F5') {
        e.preventDefault()
        loadDirectory(state.currentPath)
      } else if (e.key === 'Backspace') {
        e.preventDefault()
        goUp()
      } else if (e.key === 'Delete') {
        const selectedItem = state.items.find(i => state.selectedItems.has(i.name))
        if (selectedItem) {
          setDeleteItemState(selectedItem)
        }
      } else if (e.key === 'F2') {
        const selectedItem = state.items.find(i => state.selectedItems.has(i.name))
        if (selectedItem) {
          startRename(selectedItem)
        }
      } else if (e.ctrlKey && e.key === 'a') {
        e.preventDefault()
        setState(prev => ({
          ...prev,
          selectedItems: new Set(prev.items.map(i => i.name)),
        }))
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [activeTab, state.currentPath, state.items, state.selectedItems, loadDirectory, goUp])

  // Check if at root (for Windows path display)
  const isAtRoot = state.currentPath === '/' || state.currentPath === ''

  // Check if operation is in progress
  const isOperationInProgress = operation.loading

  return (
    <div className="fb-container">
      {/* Error Toast */}
      {toast && (
        <ErrorToast
          message={toast}
          onDismiss={() => setToast(null)}
        />
      )}

      {/* Header */}
      <div className="fb-header">
        <div className="fb-header-left">
          <h2 className="fb-title">Files</h2>
          <div className="fb-tabs">
            <button
              className={`fb-tab ${activeTab === 'browser' ? 'active' : ''}`}
              onClick={() => setActiveTab('browser')}
            >
              Browser
            </button>
            <button
              className={`fb-tab ${activeTab === 'info' ? 'active' : ''}`}
              onClick={() => setActiveTab('info')}
            >
              Info
            </button>
          </div>
        </div>
        <div className="fb-header-right">
          {activeTab === 'browser' && (
            <>
              <UploadZone
                currentPath={state.currentPath}
                onUploadComplete={() => loadDirectory(state.currentPath)}
                onError={showError}
              />
              <button
                className="fb-btn"
                onClick={() => setShowNewFolderDialog(true)}
                title="New Folder"
                disabled={isOperationInProgress}
              >
                +📁
              </button>
            </>
          )}
          <button
            className="fb-btn"
            onClick={() => loadDirectory(state.currentPath)}
            title="Refresh"
            disabled={state.loading}
          >
            ↻
          </button>
          <button
            className="fb-btn"
            onClick={() => window.open('/files/', '_blank')}
            title="Open in new tab"
          >
            ↗
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="fb-content">
        {activeTab === 'browser' ? (
          <>
            {/* Inbox Panel */}
            <InboxPanel onError={showError} />

            {/* Toolbar */}
            <div className="fb-toolbar">
              <div className="fb-toolbar-nav">
                <button
                  className="fb-nav-btn"
                  onClick={goBack}
                  disabled={historyIndex === 0}
                  title="Back"
                >
                  ←
                </button>
                <button
                  className="fb-nav-btn"
                  onClick={goForward}
                  disabled={historyIndex >= navigationHistory.length - 1}
                  title="Forward"
                >
                  →
                </button>
                <button
                  className="fb-nav-btn"
                  onClick={goUp}
                  disabled={state.currentPath === '/'}
                  title="Up"
                >
                  ↑
                </button>
              </div>
              <Breadcrumbs path={state.currentPath} onNavigate={navigateTo} />
              <div className="fb-toolbar-actions">
                <input
                  type="text"
                  className="fb-search"
                  placeholder="Filter..."
                  value={state.searchQuery}
                  onChange={(e) => setState(prev => ({ ...prev, searchQuery: e.target.value }))}
                />
                <button
                  className={`fb-view-btn ${state.viewMode === 'list' ? 'active' : ''}`}
                  onClick={() => setState(prev => ({ ...prev, viewMode: 'list' }))}
                  title="List view"
                >
                  ≡
                </button>
                <button
                  className={`fb-view-btn ${state.viewMode === 'grid' ? 'active' : ''}`}
                  onClick={() => setState(prev => ({ ...prev, viewMode: 'grid' }))}
                  title="Grid view"
                >
                  ⊞
                </button>
              </div>
            </div>

            {/* File List */}
            <div
              className="fb-list-container"
              onContextMenu={(e) => {
                if ((e.target as HTMLElement).closest('.fb-row, .fb-grid-item')) return
                handleContextMenu(null, e)
              }}
            >
              {state.loading ? (
                <div className="fb-loading">
                  <span className="fb-spinner" />
                  Loading...
                </div>
              ) : state.error ? (
                <div className="fb-error">
                  <span className="fb-error-icon">⚠</span>
                  <span className="fb-error-message">{state.error}</span>
                  <button className="fb-retry-btn" onClick={() => loadDirectory(state.currentPath)}>
                    Retry
                  </button>
                </div>
              ) : sortedItems.length === 0 ? (
                <div className="fb-empty">
                  <span className="fb-empty-icon">📂</span>
                  {state.searchQuery ? 'No matches found' : 'This folder is empty'}
                </div>
              ) : state.viewMode === 'list' ? (
                <div className="fb-list" role="grid">
                  {/* Header */}
                  <div className="fb-list-header" role="row">
                    <ColumnHeader
                      label="Name"
                      sortKey="name"
                      currentSort={state.sortBy}
                      currentDir={state.sortDir}
                      onSort={handleSort}
                      className="fb-cell-name"
                    />
                    <ColumnHeader
                      label="Size"
                      sortKey="size"
                      currentSort={state.sortBy}
                      currentDir={state.sortDir}
                      onSort={handleSort}
                      className="fb-cell-size"
                    />
                    <ColumnHeader
                      label="Modified"
                      sortKey="modified"
                      currentSort={state.sortBy}
                      currentDir={state.sortDir}
                      onSort={handleSort}
                      className="fb-cell-modified"
                    />
                  </div>
                  {/* Rows */}
                  <div className="fb-list-body">
                    {sortedItems.map(item => (
                      <FileRow
                        key={item.name}
                        item={item}
                        isSelected={state.selectedItems.has(item.name)}
                        onSelect={(e) => handleSelect(item, e)}
                        onNavigate={() => navigateTo(item.path)}
                        onContextMenu={(e) => handleContextMenu(item, e)}
                        onRename={() => startRename(item)}
                        isRenaming={renamingItem === item.name}
                        renameValue={renameValue}
                        setRenameValue={setRenameValue}
                        onRenameSubmit={submitRename}
                        onRenameCancel={cancelRename}
                        isAtRoot={isAtRoot}
                        disabled={isOperationInProgress}
                      />
                    ))}
                  </div>
                </div>
              ) : (
                <div className="fb-grid" role="grid">
                  {sortedItems.map(item => (
                    <FileGridItem
                      key={item.name}
                      item={item}
                      isSelected={state.selectedItems.has(item.name)}
                      onSelect={(e) => handleSelect(item, e)}
                      onNavigate={() => navigateTo(item.path)}
                      onContextMenu={(e) => handleContextMenu(item, e)}
                      isAtRoot={isAtRoot}
                      disabled={isOperationInProgress}
                    />
                  ))}
                </div>
              )}
            </div>

            {/* Status Bar */}
            <div className="fb-statusbar">
              <span>{sortedItems.length} items</span>
              {state.selectedItems.size > 0 && (
                <span>{state.selectedItems.size} selected</span>
              )}
              {isOperationInProgress && (
                <span className="fb-statusbar-operation">
                  {operation.type === 'rename' && 'Renaming...'}
                  {operation.type === 'delete' && 'Deleting...'}
                  {operation.type === 'create' && 'Creating...'}
                  {operation.type === 'upload' && 'Uploading...'}
                </span>
              )}
            </div>
          </>
        ) : (
          <InfoPanel />
        )}
      </div>

      {/* Context Menu */}
      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          item={contextMenu.item}
          onClose={() => setContextMenu(null)}
          onDownload={handleDownload}
          onRename={() => contextMenu.item && startRename(contextMenu.item)}
          onDelete={() => contextMenu.item && setDeleteItemState(contextMenu.item)}
          onCopyPath={handleCopyPath}
          onNewFolder={() => {
            setContextMenu(null)
            setShowNewFolderDialog(true)
          }}
        />
      )}

      {/* Dialogs */}
      {showNewFolderDialog && (
        <NewFolderDialog
          onClose={() => {
            setShowNewFolderDialog(false)
            clearOperation()
          }}
          onCreate={handleNewFolder}
          loading={operation.type === 'create' && operation.loading}
          error={operation.type === 'create' ? operation.error : null}
        />
      )}

      {deleteItemState && (
        <DeleteDialog
          item={deleteItemState}
          onClose={() => {
            setDeleteItemState(null)
            clearOperation()
          }}
          onConfirm={handleDelete}
          loading={operation.type === 'delete' && operation.loading}
          error={operation.type === 'delete' ? operation.error : null}
        />
      )}
    </div>
  )
}

export default FilesView
