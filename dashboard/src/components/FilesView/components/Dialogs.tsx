import { useState, useRef, useEffect } from 'react'
import { FileItem } from '../types'

// New Folder Dialog
interface NewFolderDialogProps {
  onClose: () => void
  onCreate: (name: string) => void
  loading?: boolean
  error?: string | null
}

export function NewFolderDialog({
  onClose,
  onCreate,
  loading,
  error,
}: NewFolderDialogProps) {
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
interface DeleteDialogProps {
  item: FileItem
  onClose: () => void
  onConfirm: () => void
  loading?: boolean
  error?: string | null
}

export function DeleteDialog({
  item,
  onClose,
  onConfirm,
  loading,
  error,
}: DeleteDialogProps) {
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
