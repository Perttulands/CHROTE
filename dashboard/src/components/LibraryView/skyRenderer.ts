/**
 * One frame of the night sky, painted on a 2D canvas.
 *
 * The component works out what each star is doing (where it is, how lit, what
 * is flaring) and hands this a flat list in screen space; this only paints.
 * Nebulae and glows are added light, so where systems crowd the sky brightens
 * rather than muddies, and the background stays the page's own.
 */

import type { StarRole } from './skyLayout'

export interface PaintedRegion {
  x: number
  y: number
  radius: number
  hue: string
  /** 0 for a subject with no evidence, 1 for the best evidenced. */
  nebula: number
  /** How much its edge shows: faint, or lit while pointed at or chosen. */
  edge: number
  alpha: number
}

export interface PaintedStar {
  x: number
  y: number
  role: StarRole
  hue: string
  size: number
  alpha: number
  /** A search found it. */
  match: boolean
  /** The star the pointer or the reader is on. */
  hot: boolean
  /** 0..1 through a birth flare, or null. */
  flare: number | null
  /** 0..1 through a freshness pulse, or null. */
  pulse: number | null
}

export interface PaintedLine {
  ax: number
  ay: number
  bx: number
  by: number
  hue: string
  alpha: number
  width: number
  /** How far the line has been drawn from a towards b, for the trace. */
  reach: number
  /** Bend sideways as a share of the length, so bridges read as arcs. */
  bend: number
}

export interface SkyFrame {
  width: number
  height: number
  regions: PaintedRegion[]
  bridges: PaintedLine[]
  links: PaintedLine[]
  stars: PaintedStar[]
  accent: string
}

/** '#rrggbb' at an alpha, for gradient stops. */
export function tint(hex: string, alpha: number): string {
  const value = /^#([0-9a-f]{6})/i.exec(hex)?.[1]
  if (!value) return `rgba(160, 170, 190, ${alpha})`
  const n = parseInt(value, 16)
  return `rgba(${n >> 16}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
}

/** A colour moved towards white, for star cores that should read as light. */
export function whiten(hex: string, share: number): string {
  const value = /^#([0-9a-f]{6})/i.exec(hex)?.[1]
  if (!value) return hex
  const n = parseInt(value, 16)
  const mix = (channel: number) => Math.round(channel + (255 - channel) * share)
  return `rgb(${mix(n >> 16)}, ${mix((n >> 8) & 255)}, ${mix(n & 255)})`
}

function line(context: CanvasRenderingContext2D, entry: PaintedLine) {
  const dx = entry.bx - entry.ax, dy = entry.by - entry.ay
  const cx = entry.ax + dx / 2 - dy * entry.bend
  const cy = entry.ay + dy / 2 + dx * entry.bend
  context.beginPath()
  context.moveTo(entry.ax, entry.ay)
  if (entry.reach >= 1) {
    context.quadraticCurveTo(cx, cy, entry.bx, entry.by)
  } else {
    // The part of the curve drawn so far, as its own smaller curve.
    const t = entry.reach
    const qx = entry.ax + (cx - entry.ax) * t, qy = entry.ay + (cy - entry.ay) * t
    const rx = cx + (entry.bx - cx) * t, ry = cy + (entry.by - cy) * t
    context.quadraticCurveTo(qx, qy, qx + (rx - qx) * t, qy + (ry - qy) * t)
  }
  context.strokeStyle = tint(entry.hue, entry.alpha)
  context.lineWidth = entry.width
  context.stroke()
}

export function paintSky(context: CanvasRenderingContext2D, frame: SkyFrame) {
  context.clearRect(0, 0, frame.width, frame.height)

  context.globalCompositeOperation = 'lighter'
  for (const region of frame.regions) {
    if (region.alpha <= 0.01) continue
    const strength = (0.035 + 0.17 * region.nebula) * region.alpha
    const glow = context.createRadialGradient(region.x, region.y, 0, region.x, region.y, region.radius * 1.08)
    glow.addColorStop(0, tint(region.hue, strength))
    glow.addColorStop(0.6, tint(region.hue, strength * 0.55))
    glow.addColorStop(1, tint(region.hue, 0))
    context.fillStyle = glow
    context.beginPath()
    context.arc(region.x, region.y, region.radius * 1.08, 0, Math.PI * 2)
    context.fill()
  }
  context.globalCompositeOperation = 'source-over'

  context.setLineDash([1, 5])
  for (const region of frame.regions) {
    if (region.alpha <= 0.01) continue
    context.beginPath()
    context.arc(region.x, region.y, region.radius, 0, Math.PI * 2)
    context.strokeStyle = tint(region.hue, region.edge * region.alpha)
    context.lineWidth = 1
    context.stroke()
  }
  context.setLineDash([])

  for (const bridge of frame.bridges) line(context, bridge)
  for (const link of frame.links) line(context, link)

  for (const star of frame.stars) {
    if (star.alpha <= 0.01) continue
    const { x, y, size } = star
    if (star.role === 'dust') {
      context.fillStyle = tint(star.hue, 0.6 * star.alpha)
      context.fillRect(x - size / 2, y - size / 2, size, size)
    } else if (star.role === 'stray') {
      context.beginPath()
      context.arc(x, y, size + 0.6, 0, Math.PI * 2)
      context.strokeStyle = tint(star.hue, 0.8 * star.alpha)
      context.lineWidth = 1
      context.stroke()
    } else {
      if (star.role === 'core' || star.hot || star.match) {
        const reach = size * (star.role === 'core' ? 5 : 4)
        context.globalCompositeOperation = 'lighter'
        const glow = context.createRadialGradient(x, y, 0, x, y, reach)
        glow.addColorStop(0, tint(star.hue, 0.55 * star.alpha))
        glow.addColorStop(1, tint(star.hue, 0))
        context.fillStyle = glow
        context.beginPath()
        context.arc(x, y, reach, 0, Math.PI * 2)
        context.fill()
        context.globalCompositeOperation = 'source-over'
      }
      context.globalAlpha = star.alpha
      context.fillStyle = whiten(star.hue, star.role === 'core' ? 0.75 : 0.35)
      context.beginPath()
      context.arc(x, y, size, 0, Math.PI * 2)
      context.fill()
      context.globalAlpha = 1
    }
    if (star.match) {
      context.beginPath()
      context.arc(x, y, size + 4, 0, Math.PI * 2)
      context.strokeStyle = tint(frame.accent, 0.9 * star.alpha)
      context.lineWidth = 1.2
      context.stroke()
    }
    if (star.hot) {
      context.beginPath()
      context.arc(x, y, size + 6, 0, Math.PI * 2)
      context.strokeStyle = whiten(star.hue, 0.5)
      context.lineWidth = 1
      context.stroke()
    }
    if (star.flare !== null) {
      const t = star.flare
      context.beginPath()
      context.arc(x, y, size + 2 + t * 16, 0, Math.PI * 2)
      context.strokeStyle = tint(star.hue, (1 - t) * 0.9)
      context.lineWidth = 1.5 * (1 - t) + 0.3
      context.stroke()
    }
    if (star.pulse !== null) {
      const t = star.pulse
      context.beginPath()
      context.arc(x, y, size + 2 + t * 9, 0, Math.PI * 2)
      context.strokeStyle = tint(star.hue, (1 - t) * 0.55 * star.alpha)
      context.lineWidth = 1
      context.stroke()
    }
  }
}
