// ============================================
// FILE OPERATION ERROR
// ============================================

export type FileErrorCode =
  | 'NETWORK'
  | 'NOT_FOUND'
  | 'PERMISSION'
  | 'CONFLICT'
  | 'INVALID'
  | 'SERVER'
  | 'STORAGE'

export class FileOperationError extends Error {
  constructor(
    message: string,
    public code: FileErrorCode,
    public status?: number
  ) {
    super(message)
    this.name = 'FileOperationError'
  }
}

// ============================================
// FILE ITEM TYPES
// ============================================

export interface FileItem {
  name: string
  size: number
  modified: string
  isDir: boolean
  type: string
  path: string
}

export interface RawFileItem {
  name: string
  size: number
  modified: string
  isDir: boolean
  type: string
}

export interface DirectoryResponse {
  isDir: boolean
  items?: RawFileItem[]
  name?: string
}

export interface FilesViewProps {
  navigateRequest?: { path: string; nonce: number } | null
  /** The session a path is sent to: the terminal last in front. */
  sendTarget?: string | null
}
