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

// ============================================
// STATE TYPES
// ============================================

export interface FileBrowserState {
  items: FileItem[]
  loading: boolean
  error: string | null
  currentPath: string
  selectedItems: Set<string>
  sortBy: 'name' | 'size' | 'modified'
  sortDir: 'asc' | 'desc'
  viewMode: 'list' | 'grid'
  searchQuery: string
}

export type OperationType = 'rename' | 'delete' | 'create' | 'upload' | null

export interface OperationState {
  type: OperationType
  loading: boolean
  error: string | null
  target?: string
}

export type ViewTab = 'browser' | 'info'

// ============================================
// CONTEXT MENU TYPES
// ============================================

export interface ContextMenuState {
  x: number
  y: number
  item: FileItem | null
}

// ============================================
// PATH MAPPING
// ============================================

export const PATH_MAP: Record<string, string> = {
  '/code': 'E:/Code',
  '/vault': 'E:/Vault',
}

/**
 * Convert container path to display path (Windows path)
 */
export function toDisplayPath(containerPath: string): string {
  for (const [container, windows] of Object.entries(PATH_MAP)) {
    if (containerPath.startsWith(container)) {
      return containerPath.replace(container, windows)
    }
  }
  return containerPath
}

/**
 * Convert display path (Windows) to container path
 */
export function toContainerPath(displayPath: string): string {
  for (const [container, windows] of Object.entries(PATH_MAP)) {
    if (displayPath.startsWith(windows)) {
      return displayPath.replace(windows, container)
    }
  }
  return displayPath
}

/**
 * Get display name for root items
 * When at root, show E:/Code instead of 'code'
 */
export function getRootDisplayName(name: string): string {
  const mapping: Record<string, string> = {
    'code': 'E:/Code',
    'vault': 'E:/Vault',
  }
  return mapping[name] || name
}
