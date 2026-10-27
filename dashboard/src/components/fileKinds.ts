/**
 * What a file is, from its path: its name, its extension, and the kind of
 * preview the Files panel can give it before reading a byte.
 */

import type { FileItem } from './FilesView/types'

export type PreviewKind = 'text' | 'image' | 'audio' | 'video' | 'pdf' | 'model' | 'download'

const TEXT_EXTENSIONS = new Set([
  'adoc', 'asm', 'bash', 'bat', 'c', 'cc', 'cfg', 'clj', 'cljs', 'cljc', 'cmake', 'cnf',
  'conf', 'cpp', 'cs', 'css', 'csv', 'cts', 'cue', 'dart', 'desktop', 'diff',
  'dockerfile', 'dockerignore', 'editorconfig', 'eml', 'env', 'erl', 'ex', 'exs', 'fish',
  'fs', 'fsx', 'geojson', 'gitattributes', 'gitconfig', 'gitignore', 'gitmodules', 'go',
  'gql', 'gradle', 'graphql', 'groovy', 'h', 'har', 'hcl', 'hpp', 'hs', 'htm', 'html',
  'ics', 'ignore', 'ini', 'ipynb', 'java', 'jl', 'js', 'json', 'json5', 'jsonc', 'jsonl',
  'jsx', 'kt', 'kts', 'less',
  'lock', 'log', 'lua', 'm', 'markdown', 'md', 'mjs', 'mount',
  'ndjson', 'nim', 'nix', 'npmrc', 'patch', 'path', 'pem', 'php', 'pl',
  'properties', 'proto', 'ps1', 'psv', 'py', 'r', 'rb', 'rego', 'rs', 'rst', 's', 'sass',
  'scala', 'scss', 'service', 'sh', 'socket', 'sql', 'svelte',
  'swift', 'target', 'tex', 'tf', 'tfstate', 'tfvars', 'timer', 'toml', 'ts',
  'tsbuildinfo', 'tsv', 'tsx', 'txt', 'vcf', 'vue', 'webmanifest', 'xml',
  'yaml', 'yml', 'zsh',
])
const TEXT_FILE_NAMES = new Set([
  'brewfile', 'gemfile', 'go.mod', 'go.sum', 'go.work', 'jenkinsfile', 'justfile', 'license',
  'makefile', 'pipfile', 'procfile', 'rakefile', 'readme', 'vagrantfile',
])
const IMAGE_EXTENSIONS = new Set(['avif', 'bmp', 'gif', 'ico', 'jpeg', 'jpg', 'png', 'svg', 'webp'])
const AUDIO_EXTENSIONS = new Set(['aac', 'flac', 'm4a', 'mp3', 'oga', 'ogg', 'opus', 'wav'])
const VIDEO_EXTENSIONS = new Set(['avi', 'm4v', 'mkv', 'mov', 'mp4', 'mpeg', 'mpg', 'ogv', 'webm'])
export function normalizeFilePath(path: string): string {
  const trimmed = path.trim()
  if (!trimmed || trimmed === '.') return '/'
  const withRoot = trimmed.startsWith('/') ? trimmed : `/${trimmed}`
  const compact = withRoot.replace(/\/+/g, '/')
  return compact.length > 1 ? compact.replace(/\/$/, '') : compact
}

export function getFileBaseName(path: string): string {
  const parts = normalizeFilePath(path).split('/').filter(Boolean)
  return parts[parts.length - 1] || '/'
}

export function getFileExtension(name: string): string {
  const lower = name.toLowerCase()
  if (lower === 'dockerfile' || lower.startsWith('dockerfile.')) return 'dockerfile'
  if (lower.startsWith('.env')) return 'env'
  const parts = lower.split('.')
  return parts.length > 1 ? parts.pop() || '' : ''
}

export function isMarkdownFileName(name: string): boolean {
  const extension = getFileExtension(name)
  return extension === 'md' || extension === 'markdown'
}

export function getPreviewKind(item: FileItem): PreviewKind {
  const extension = getFileExtension(item.name)
  const lower = item.name.toLowerCase()
  if (TEXT_EXTENSIONS.has(extension) || TEXT_FILE_NAMES.has(lower)) return 'text'
  if (IMAGE_EXTENSIONS.has(extension)) return 'image'
  if (AUDIO_EXTENSIONS.has(extension)) return 'audio'
  if (VIDEO_EXTENSIONS.has(extension)) return 'video'
  if (extension === 'glb') return 'model'
  if (extension === 'pdf') return 'pdf'
  return 'download'
}

export function makeFileItemFromPath(path: string): FileItem {
  const name = getFileBaseName(path)
  return {
    name,
    path: normalizeFilePath(path),
    isDir: false,
    size: 0,
    modified: new Date().toISOString(),
    type: getFileExtension(name),
  }
}
