import { FileItem, FileOperationError, DirectoryResponse, RawFileItem } from './types'

const API_BASE = '/api/files'

// ============================================
// FILENAME SANITIZATION (security)
// ============================================

/**
 * Sanitize filename to prevent path traversal and other attacks
 * SECURITY: Rejects dangerous patterns, allows only safe characters
 */
function sanitizeFilename(filename: string): string {
  // Reject path separators and traversal patterns
  if (filename.includes('/') || filename.includes('\\') || filename.includes('..')) {
    throw new FileOperationError(
      'Invalid filename: path separators and ".." are not allowed',
      'INVALID'
    )
  }

  // Reject null bytes and control characters
  if (/[\x00-\x1f\x7f]/.test(filename)) {
    throw new FileOperationError(
      'Invalid filename: control characters are not allowed',
      'INVALID'
    )
  }

  // Reject empty or whitespace-only names
  const trimmed = filename.trim()
  if (!trimmed) {
    throw new FileOperationError('Invalid filename: name cannot be empty', 'INVALID')
  }

  // Reject names that are only dots
  if (/^\.+$/.test(trimmed)) {
    throw new FileOperationError('Invalid filename: "." and ".." are not allowed', 'INVALID')
  }

  return trimmed
}

// ============================================
// ERROR HANDLING
// ============================================

/**
 * Throws a FileOperationError based on HTTP status code
 * No silent failures - every error surfaces
 */
function throwForStatus(response: Response, context: string): void {
  if (response.ok) return

  switch (response.status) {
    case 403:
      throw new FileOperationError('Permission denied', 'PERMISSION', 403)
    case 404:
      throw new FileOperationError('File not found', 'NOT_FOUND', 404)
    case 409:
      throw new FileOperationError('Already exists', 'CONFLICT', 409)
    case 413:
      throw new FileOperationError('File too large', 'STORAGE', 413)
    case 507:
      throw new FileOperationError('Insufficient storage', 'STORAGE', 507)
    default:
      throw new FileOperationError(`${context}: ${response.status}`, 'SERVER', response.status)
  }
}

/**
 * Get user-friendly error message for operations
 */
export function getErrorMessage(error: unknown, context: string): string {
  if (error instanceof FileOperationError) {
    switch (error.code) {
      case 'PERMISSION':
        return 'Permission denied'
      case 'NOT_FOUND':
        return context === 'rename' ? 'File not found' : 'Path not found'
      case 'CONFLICT':
        return context === 'create' ? 'Folder already exists' : 'Destination already exists'
      case 'STORAGE':
        return error.status === 413 ? 'File too large' : 'Insufficient storage'
      case 'INVALID':
        return error.message
      case 'NETWORK':
        return 'Network error - check your connection'
      case 'SERVER':
        return `Server error (${error.status})`
      default:
        return error.message
    }
  }
  if (error instanceof Error) {
    return error.message
  }
  return 'An unexpected error occurred'
}

// ============================================
// API FUNCTIONS - No fallbacks, throw on any error
// ============================================

/**
 * Fetch directory contents
 * Throws on: non-200 response, non-directory, malformed response, network error
 */
export async function fetchDirectory(path: string): Promise<FileItem[]> {
  const cleanPath = path.startsWith('/') ? path : '/' + path

  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${cleanPath}`, {
      headers: {
        'Accept': 'application/json',
      },
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  throwForStatus(response, 'Failed to fetch directory')

  let data: DirectoryResponse
  try {
    data = await response.json()
  } catch {
    throw new FileOperationError('Invalid server response', 'INVALID')
  }

  // Validate response structure - NO silent fallback to []
  if (typeof data !== 'object' || data === null) {
    throw new FileOperationError('Invalid directory response', 'INVALID')
  }

  if (data.isDir === false) {
    throw new FileOperationError('Path is not a directory', 'INVALID')
  }

  if (!('items' in data) || !Array.isArray(data.items)) {
    throw new FileOperationError('Invalid directory response', 'INVALID')
  }

  return data.items.map((item: RawFileItem) => ({
    name: item.name,
    size: item.size,
    modified: item.modified,
    isDir: item.isDir,
    type: item.type,
    path: cleanPath === '/' ? `/${item.name}` : `${cleanPath}/${item.name}`,
  }))
}

/**
 * Create a new folder
 * Throws on: 403 (permission), 409 (exists), 500+ (server error)
 */
export async function createFolder(path: string, name: string): Promise<void> {
  const fullPath = path === '/' ? `/${name}` : `${path}/${name}`

  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${fullPath}/`, {
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
      },
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  if (response.status === 409) {
    throw new FileOperationError('Folder already exists', 'CONFLICT', 409)
  }

  throwForStatus(response, 'Failed to create folder')
}

/**
 * Rename/move a file or folder
 * Throws on: 404 (not found), 409 (destination exists), 403 (permission)
 */
export async function renameItem(oldPath: string, newPath: string): Promise<void> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${oldPath}`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        action: 'rename',
        destination: newPath,
      }),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  if (response.status === 409) {
    throw new FileOperationError('Destination already exists', 'CONFLICT', 409)
  }

  throwForStatus(response, 'Failed to rename')
}

/**
 * Delete a file or folder
 * Throws on: 403 (permission), 404 (not found)
 */
export async function deleteItem(path: string): Promise<void> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${path}`, {
      method: 'DELETE',
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  throwForStatus(response, 'Failed to delete')
}

/**
 * Upload files to a directory
 * Throws on: 413 (too large), 507 (storage), 403 (permission), invalid filename
 * SECURITY: Sanitizes filenames to prevent path traversal
 */
export async function uploadFiles(path: string, files: FileList | File[]): Promise<void> {
  const fileArray = Array.isArray(files) ? files : Array.from(files)

  for (const file of fileArray) {
    // SECURITY: Sanitize filename before building path
    const safeName = sanitizeFilename(file.name)
    const fullPath = path === '/' ? `/${safeName}` : `${path}/${safeName}`

    let response: Response
    try {
      response = await fetch(`${API_BASE}/resources${fullPath}`, {
        method: 'POST',
        headers: {
          'Content-Type': file.type || 'application/octet-stream',
        },
        body: file,
      })
    } catch (error) {
      throw new FileOperationError(
        error instanceof Error ? error.message : 'Network error',
        'NETWORK'
      )
    }

    if (response.status === 413) {
      throw new FileOperationError('File too large', 'STORAGE', 413)
    }

    if (response.status === 507) {
      throw new FileOperationError('Insufficient storage', 'STORAGE', 507)
    }

    throwForStatus(response, `Failed to upload ${safeName}`)
  }
}

/**
 * Check if a path exists (used by InboxPanel)
 * Returns true/false, does not throw on 404
 */
export async function pathExists(path: string): Promise<boolean> {
  try {
    const response = await fetch(`${API_BASE}/resources${path}`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
      },
    })
    return response.ok
  } catch {
    return false
  }
}

/**
 * Get download URL for a file
 */
export function getDownloadUrl(path: string): string {
  return `${API_BASE}/raw${path}?inline=false`
}
