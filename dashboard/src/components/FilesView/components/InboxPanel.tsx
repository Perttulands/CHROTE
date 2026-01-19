import { useState, useRef } from 'react'
import { uploadFiles, uploadFilesWithPaths, pathExists, createFolder, getErrorMessage, FileWithPath } from '../fileService'
import { formatSize } from '../utils'

interface InboxPanelProps {
  onError: (message: string) => void
}

// Item can be a file or a folder (folder represented by its files with paths)
interface InboxItem {
  type: 'file' | 'folder'
  name: string
  size: number
  file?: File // for single files
  filesWithPaths?: FileWithPath[] // for folders
}

// Helper to read all files from a directory entry recursively
async function readDirectoryEntry(entry: FileSystemDirectoryEntry, basePath: string = ''): Promise<FileWithPath[]> {
  const results: FileWithPath[] = []
  const reader = entry.createReader()

  const readEntries = (): Promise<FileSystemEntry[]> => {
    return new Promise((resolve, reject) => {
      reader.readEntries(resolve, reject)
    })
  }

  // readEntries may not return all entries at once, need to call until empty
  let entries: FileSystemEntry[] = []
  let batch: FileSystemEntry[]
  do {
    batch = await readEntries()
    entries = entries.concat(batch)
  } while (batch.length > 0)

  for (const childEntry of entries) {
    const childPath = basePath ? `${basePath}/${childEntry.name}` : childEntry.name

    if (childEntry.isFile) {
      const fileEntry = childEntry as FileSystemFileEntry
      const file = await new Promise<File>((resolve, reject) => {
        fileEntry.file(resolve, reject)
      })
      results.push({ file, relativePath: childPath })
    } else if (childEntry.isDirectory) {
      const subResults = await readDirectoryEntry(childEntry as FileSystemDirectoryEntry, childPath)
      results.push(...subResults)
    }
  }

  return results
}

// Process DataTransfer items to extract files and folders
async function processDataTransferItems(items: DataTransferItemList): Promise<InboxItem[]> {
  const results: InboxItem[] = []

  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    if (item.kind !== 'file') continue

    const entry = item.webkitGetAsEntry?.()
    if (!entry) {
      // Fallback for browsers without webkitGetAsEntry
      const file = item.getAsFile()
      if (file) {
        results.push({ type: 'file', name: file.name, size: file.size, file })
      }
      continue
    }

    if (entry.isFile) {
      const file = item.getAsFile()
      if (file) {
        results.push({ type: 'file', name: file.name, size: file.size, file })
      }
    } else if (entry.isDirectory) {
      const filesWithPaths = await readDirectoryEntry(entry as FileSystemDirectoryEntry, entry.name)
      const totalSize = filesWithPaths.reduce((sum, f) => sum + f.file.size, 0)
      results.push({
        type: 'folder',
        name: entry.name,
        size: totalSize,
        filesWithPaths
      })
    }
  }

  return results
}

export function InboxPanel({ onError }: InboxPanelProps) {
  const [items, setItems] = useState<InboxItem[]>([])
  const [note, setNote] = useState('')
  const [isDragging, setIsDragging] = useState(false)
  const [sending, setSending] = useState(false)
  const [status, setStatus] = useState<'idle' | 'success' | 'error'>('idle')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const folderInputRef = useRef<HTMLInputElement>(null)

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

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)

    if (e.dataTransfer.items && e.dataTransfer.items.length > 0) {
      const newItems = await processDataTransferItems(e.dataTransfer.items)
      if (newItems.length > 0) {
        setItems(prev => [...prev, ...newItems])
        setStatus('idle')
      }
    } else if (e.dataTransfer.files.length > 0) {
      // Fallback for browsers without items API
      const newItems: InboxItem[] = Array.from(e.dataTransfer.files).map(file => ({
        type: 'file' as const,
        name: file.name,
        size: file.size,
        file
      }))
      setItems(prev => [...prev, ...newItems])
      setStatus('idle')
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      const newItems: InboxItem[] = Array.from(e.target.files).map(file => ({
        type: 'file' as const,
        name: file.name,
        size: file.size,
        file
      }))
      setItems(prev => [...prev, ...newItems])
      setStatus('idle')
    }
  }

  const handleFolderSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      const fileList = Array.from(e.target.files)
      // Files from folder input have webkitRelativePath
      // Group them by top-level folder
      const folderMap = new Map<string, FileWithPath[]>()

      for (const file of fileList) {
        const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name
        const topFolder = relativePath.split('/')[0]

        if (!folderMap.has(topFolder)) {
          folderMap.set(topFolder, [])
        }
        folderMap.get(topFolder)!.push({ file, relativePath })
      }

      const newItems: InboxItem[] = Array.from(folderMap.entries()).map(([folderName, filesWithPaths]) => ({
        type: 'folder' as const,
        name: folderName,
        size: filesWithPaths.reduce((sum, f) => sum + f.file.size, 0),
        filesWithPaths
      }))

      setItems(prev => [...prev, ...newItems])
      setStatus('idle')
    }
  }

  const removeItem = (index: number) => {
    setItems(prev => prev.filter((_, i) => i !== index))
  }

  const handleSend = async () => {
    if (items.length === 0) return
    setSending(true)

    try {
      // Ensure incoming folder exists
      const exists = await pathExists(INBOX_PATH)
      if (!exists) {
        await createFolder('/code', 'incoming')
      }

      // Upload all items
      for (const item of items) {
        if (item.type === 'file' && item.file) {
          await uploadFiles(INBOX_PATH, [item.file])
        } else if (item.type === 'folder' && item.filesWithPaths) {
          await uploadFilesWithPaths(INBOX_PATH, item.filesWithPaths)
        }
      }

      // Upload note file if provided
      if (note.trim() && items.length > 0) {
        const noteFile = new File([note.trim()], `${items[0].name}.note`, { type: 'text/plain' })
        await uploadFiles(INBOX_PATH, [noteFile])
      }

      setStatus('success')
      setItems([])
      setNote('')
      if (fileInputRef.current) fileInputRef.current.value = ''
      if (folderInputRef.current) folderInputRef.current.value = ''
      setTimeout(() => setStatus('idle'), 2000)
    } catch (error) {
      setStatus('error')
      const message = getErrorMessage(error, 'upload')
      onError(`Failed to send: ${message}`)
      setTimeout(() => setStatus('idle'), 2000)
    } finally {
      setSending(false)
    }
  }

  const totalSize = items.reduce((sum, item) => sum + item.size, 0)
  const totalFiles = items.reduce((sum, item) => {
    if (item.type === 'file') return sum + 1
    return sum + (item.filesWithPaths?.length || 0)
  }, 0)

  const folderCount = items.filter(i => i.type === 'folder').length
  const fileCount = items.filter(i => i.type === 'file').length

  return (
    <div className={`inbox-panel ${isDragging ? 'dragging' : ''} ${status} ${items.length > 0 ? 'has-files' : ''}`}>
      <div
        className="inbox-dropzone"
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          onChange={handleFileSelect}
          style={{ display: 'none' }}
        />
        <input
          ref={folderInputRef}
          type="file"
          // @ts-expect-error webkitdirectory is not in React types but works in browsers
          webkitdirectory=""
          onChange={handleFolderSelect}
          style={{ display: 'none' }}
        />
        {items.length > 0 ? (
          <div className="inbox-files-list">
            {items.map((item, index) => (
              <div key={`${item.name}-${index}`} className="inbox-file-item">
                <span className="inbox-file-icon">{item.type === 'folder' ? '📁' : '📄'}</span>
                <span className="inbox-file-name">
                  {item.name}
                  {item.type === 'folder' && item.filesWithPaths && (
                    <span className="inbox-file-count"> ({item.filesWithPaths.length} files)</span>
                  )}
                </span>
                <span className="inbox-file-size">{formatSize(item.size)}</span>
                <button
                  className="inbox-file-remove"
                  onClick={(e) => { e.stopPropagation(); removeItem(index) }}
                >
                  ×
                </button>
              </div>
            ))}
            <div className="inbox-add-buttons">
              <button
                className="inbox-add-more"
                onClick={(e) => { e.stopPropagation(); fileInputRef.current?.click() }}
              >
                + Files
              </button>
              <button
                className="inbox-add-more"
                onClick={(e) => { e.stopPropagation(); folderInputRef.current?.click() }}
              >
                + Folder
              </button>
            </div>
          </div>
        ) : (
          <div className="inbox-placeholder">
            <span className="inbox-icon">📬</span>
            <span className="inbox-title">Send a package to /code/incoming</span>
            <span className="inbox-subtitle">Drop files or folders here</span>
            <div className="inbox-browse-buttons">
              <button
                className="inbox-browse-btn"
                onClick={(e) => { e.stopPropagation(); fileInputRef.current?.click() }}
              >
                Browse Files
              </button>
              <button
                className="inbox-browse-btn"
                onClick={(e) => { e.stopPropagation(); folderInputRef.current?.click() }}
              >
                Browse Folder
              </button>
            </div>
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
          {items.length > 0 && (
            <span className="inbox-summary">
              {folderCount > 0 && `${folderCount} folder${folderCount > 1 ? 's' : ''}`}
              {folderCount > 0 && fileCount > 0 && ', '}
              {fileCount > 0 && `${fileCount} file${fileCount > 1 ? 's' : ''}`}
              {' '}({totalFiles} total) · {formatSize(totalSize)}
            </span>
          )}
          <button
            className="inbox-send"
            onClick={handleSend}
            disabled={items.length === 0 || sending}
          >
            {sending ? 'Sending...' : status === 'success' ? '✓ Sent!' : `Send ${items.length > 0 ? `(${items.length})` : ''}`}
          </button>
        </div>
      </div>
    </div>
  )
}
