/**
 * The size rule for floating windows, and where a resized one is remembered.
 *
 * A floating window is sized by its content until the operator resizes it
 * once. From then on the remembered size for that kind of window decides, on
 * this device, for every later window of the kind — the operator sized the
 * window, not the picture or the session inside it. The memory is cleared
 * from the window's own header, so a size dragged into uselessness on a small
 * screen is never permanent.
 *
 * The minimum belongs to the gesture, not to the default: a content-derived
 * size smaller than the minimum is the content's honest size (a three-pixel
 * picture is three pixels wide), while a size the operator drags is held at
 * the minimum so the window cannot be dragged away to nothing.
 */

export type FloatingWindowKind = 'image' | 'peek'

export interface FrameSize {
  width: number
  height: number
}

const STORAGE_KEY = 'chrote.floatingWindowSize.v1'
const STORAGE_VERSION = 1

/** The smallest a dragged window of each kind may be: a header and a look. */
export const FLOATING_WINDOW_MINIMUM: Record<FloatingWindowKind, FrameSize> = {
  image: { width: 240, height: 120 },
  peek: { width: 240, height: 120 },
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function readSizes(): Record<string, unknown> {
  if (typeof window === 'undefined') return {}
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) || 'null')
    if (!isRecord(parsed) || parsed.version !== STORAGE_VERSION || !isRecord(parsed.sizes)) return {}
    return parsed.sizes
  } catch {
    return {}
  }
}

function writeSizes(sizes: Record<string, unknown>): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ version: STORAGE_VERSION, sizes }))
  } catch {
    // Private mode and quota failures must not make a window unopenable.
  }
}

function sanitizeSize(value: unknown): FrameSize | null {
  if (!isRecord(value)) return null
  const { width, height } = value
  if (typeof width !== 'number' || typeof height !== 'number') return null
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null
  if (width <= 0 || height <= 0) return null
  return { width, height }
}

/** The size the operator last dragged this kind of window to, if any. */
export function readFloatingWindowSize(kind: FloatingWindowKind): FrameSize | null {
  return sanitizeSize(readSizes()[kind])
}

export function writeFloatingWindowSize(kind: FloatingWindowKind, size: FrameSize): void {
  const sanitized = sanitizeSize(size)
  if (!sanitized) return
  writeSizes({ ...readSizes(), [kind]: { width: Math.round(sanitized.width), height: Math.round(sanitized.height) } })
}

export function clearFloatingWindowSize(kind: FloatingWindowKind): void {
  const sizes = readSizes()
  if (!(kind in sizes)) return
  delete sizes[kind]
  writeSizes(sizes)
}

function clampSide(length: number, minimum: number, maximum: number): number {
  const floor = Math.max(1, Number.isFinite(minimum) ? minimum : 1)
  const ceiling = Math.max(floor, Number.isFinite(maximum) ? maximum : Number.POSITIVE_INFINITY)
  return Math.min(ceiling, Math.max(floor, Number.isFinite(length) ? length : floor))
}

/**
 * Hold a size the operator is dragging inside the workspace and above the
 * minimum. The minimum wins over a workspace smaller than it, because a window
 * clipped by its own workspace is still readable and a window of nothing is
 * not.
 */
export function clampFrameSize(size: FrameSize, minimum: FrameSize, bounds: FrameSize | null): FrameSize {
  return {
    width: clampSide(size.width, minimum.width, bounds ? bounds.width : Number.POSITIVE_INFINITY),
    height: clampSide(size.height, minimum.height, bounds ? bounds.height : Number.POSITIVE_INFINITY),
  }
}

export interface FrameSizeInputs {
  /** What the operator dragged this kind of window to, on this device. */
  remembered: FrameSize | null
  /** What the content asks for, once it knows; null while it does not. */
  content: FrameSize | null
  minimum: FrameSize
  /** The workspace the window is centred in; null while it is unmeasured. */
  bounds: FrameSize | null
}

/**
 * The size to draw: the remembered one when there is one, held inside the
 * workspace and above the minimum; otherwise the content's, capped by the
 * workspace but never pushed up to the minimum; otherwise nothing yet.
 */
export function resolveFrameSize({ remembered, content, minimum, bounds }: FrameSizeInputs): FrameSize | null {
  if (remembered) return clampFrameSize(remembered, minimum, bounds)
  if (!content) return null
  if (!bounds) return content
  return {
    width: Math.min(content.width, bounds.width),
    height: Math.min(content.height, bounds.height),
  }
}
