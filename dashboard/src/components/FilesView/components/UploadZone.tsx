import { useState, useRef } from 'react'
import { uploadFiles, getErrorMessage } from '../fileService'

interface UploadZoneProps {
  currentPath: string
  onUploadComplete: () => void
  onError: (message: string) => void
}

export function UploadZone({
  currentPath,
  onUploadComplete,
  onError,
}: UploadZoneProps) {
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
