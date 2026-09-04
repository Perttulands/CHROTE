/**
 * The hairlines, drawn by the GPU.
 *
 * Measured on this host while the canvas map landed: a corpus of ten thousand
 * pages and thirty thousand links pans at a p95 of 125 ms a frame and hovers in
 * 86 ms, against a target of 16 and 4. The same corpus with no hairlines at all
 * pans at the display's own rate, so nothing about the layout, the index, the
 * light or the dots is what costs — what costs is Canvas 2D rasterizing tens of
 * thousands of long antialiased hairlines, one at a time, on the thread the
 * interface answers on.
 *
 * So the hairlines move to the GPU and nothing else does. One program, one
 * draw call for the whole corpus: each link is an instance carrying its two
 * ends, the control point of its curve, its colour and whether it is a shared
 * tag, and the shader walks the quadratic, spreads it into a strip a hairline
 * wide, feathers its edges and dashes it. A pan is then a change of two
 * uniforms; a hover is a rewrite of one byte of colour per link. Neither one
 * touches the geometry, which is what makes both of them cost the arithmetic
 * rather than the corpus.
 *
 * WebGL 2 can be missing — a browser without it, a headless run without a GPU,
 * a test in jsdom — and the map must still draw. This module answers null in
 * that case and the Canvas 2D renderer strokes the same curves itself. Nothing
 * above here needs to know which of the two drew.
 */

import { CURVE_STEPS, curveControl } from './mapCurve'
import type { MapEdge, MapNode } from './mapLayout'

/** What the layer is asked to draw, in the drawing's own coordinates. */
export interface EdgeFrame {
  nodes: readonly MapNode[]
  edges: readonly MapEdge[]
  /** The colour each link is drawn in, four bytes a link: r, g, b, a. */
  colours: Uint8Array
  width: number
  height: number
  scale: number
  offsetX: number
  offsetY: number
  ratio: number
}

export interface EdgeLayer {
  draw(frame: EdgeFrame): void
  destroy(): void
}

/**
 * Half the width a hairline is drawn at, in the box's own pixels, and how much
 * of that is solid before the edge is feathered away. One pixel of line with
 * half a pixel of feather each side is a hairline that stays a hairline at any
 * zoom, which is what a map of a corpus this size needs it to be.
 */
const HALF_WIDTH = 1
const CORE = 0.5

/** The dash a shared tag is drawn with, in the box's pixels: 2 on, 3 off. */
const DASH_ON = 2
const DASH_PERIOD = 5

/** Floats per link in the geometry buffer: two ends, a control point. */
const GEOMETRY_STRIDE = 7

const VERTEX_SHADER = `#version 300 es
precision highp float;

// One strip of CURVE_STEPS pieces, walked twice: t along the curve, side across
// it. The strip is the same for every link, so the geometry buffer holds one
// copy of it and the corpus rides in the instance buffer.
in vec2 a_walk;

in vec2 a_from;
in vec2 a_control;
in vec2 a_to;
in float a_dashed;
in vec4 a_colour;

uniform vec2 u_resolution;
uniform float u_scale;
uniform vec2 u_offset;
uniform float u_half;

out float v_side;
out float v_along;
out float v_dashed;
out vec4 v_colour;

vec2 screen(vec2 world) {
  return world * u_scale + u_offset;
}

void main() {
  float t = a_walk.x;
  float side = a_walk.y;
  float u = 1.0 - t;

  vec2 point = u * u * a_from + 2.0 * u * t * a_control + t * t * a_to;
  vec2 slope = 2.0 * u * (a_control - a_from) + 2.0 * t * (a_to - a_control);

  vec2 here = screen(point);
  vec2 direction = slope * u_scale;
  float run = length(direction);
  // A link whose ends have landed on each other has no direction to spread
  // across; it is drawn as a dot's worth of line rather than as a NaN.
  vec2 normal = run > 0.0001 ? vec2(-direction.y, direction.x) / run : vec2(0.0, 1.0);

  vec2 pixel = here + normal * side * u_half;
  gl_Position = vec4(pixel / u_resolution * 2.0 - 1.0, 0.0, 1.0);
  gl_Position.y = -gl_Position.y;

  v_side = side;
  v_dashed = a_dashed;
  v_colour = a_colour;
  // How far along the link this vertex sits, in the box's pixels, so a dash is
  // the same length however far in the map is taken. The chord is close enough
  // to the curve at the bow the map draws for the eye to read the dashes as
  // even.
  v_along = t * distance(screen(a_from), screen(a_to));
}
`

const FRAGMENT_SHADER = `#version 300 es
precision highp float;

in float v_side;
in float v_along;
in float v_dashed;
in vec4 v_colour;

uniform float u_half;
uniform float u_core;
uniform float u_dashOn;
uniform float u_dashPeriod;

out vec4 outColour;

void main() {
  if (v_dashed > 0.5 && fract(v_along / u_dashPeriod) * u_dashPeriod > u_dashOn) discard;
  // The line's own antialiasing: solid to the core, faded to nothing at the
  // edge of the strip. The browser's multisampling is not asked for, so the
  // hairline looks the same whether or not the host granted it.
  float edge = 1.0 - smoothstep(u_core / u_half, 1.0, abs(v_side));
  float alpha = v_colour.a * edge;
  if (alpha <= 0.0) discard;
  outColour = vec4(v_colour.rgb * alpha, alpha);
}
`

function compile(gl: WebGL2RenderingContext, kind: number, source: string): WebGLShader | null {
  const shader = gl.createShader(kind)
  if (!shader) return null
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  if (gl.getShaderParameter(shader, gl.COMPILE_STATUS)) return shader
  gl.deleteShader(shader)
  return null
}

function link(gl: WebGL2RenderingContext): WebGLProgram | null {
  const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX_SHADER)
  const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER)
  if (!vertex || !fragment) return null
  const program = gl.createProgram()
  if (!program) return null
  gl.attachShader(program, vertex)
  gl.attachShader(program, fragment)
  gl.linkProgram(program)
  gl.deleteShader(vertex)
  gl.deleteShader(fragment)
  if (gl.getProgramParameter(program, gl.LINK_STATUS)) return program
  gl.deleteProgram(program)
  return null
}

/** The strip every link is drawn as: t across the curve, side across the line. */
function stripWalk(): Float32Array {
  const walk = new Float32Array((CURVE_STEPS + 1) * 4)
  for (let step = 0; step <= CURVE_STEPS; step++) {
    const t = step / CURVE_STEPS
    walk[step * 4] = t
    walk[step * 4 + 1] = -1
    walk[step * 4 + 2] = t
    walk[step * 4 + 3] = 1
  }
  return walk
}

/**
 * The names a rasterizer answers to when there is no graphics card behind it.
 *
 * A browser without a GPU still offers WebGL, drawn by the processor. Measured
 * here on a host with no card: the same corpus that costs Canvas 2D a p95 of
 * 315 ms a frame costs the GPU path 1345 ms, because a software rasterizer pays
 * for every pixel of every one of the strips a hairline is spread into. So the
 * layer asks what is behind it and stands down when the answer is a program,
 * which leaves the canvas to draw exactly as it did before.
 */
const SOFTWARE = ['swiftshader', 'llvmpipe', 'softpipe', 'software', 'basic render']

function drawnBySoftware(gl: WebGL2RenderingContext): boolean {
  const info = gl.getExtension('WEBGL_debug_renderer_info')
  const name = String(
    (info && gl.getParameter(info.UNMASKED_RENDERER_WEBGL)) || gl.getParameter(gl.RENDERER) || '',
  ).toLowerCase()
  return SOFTWARE.some(mark => name.includes(mark))
}

/**
 * The layer, or null when this browser has no GPU to draw on. A null is not a
 * failure: it is the map saying the Canvas 2D path is what will draw, which at
 * the corpus size a reader has is the same picture at the same speed.
 */
export function createEdgeLayer(canvas: HTMLCanvasElement): EdgeLayer | null {
  let gl: WebGL2RenderingContext | null = null
  try {
    gl = canvas.getContext('webgl2', {
      alpha: true,
      antialias: false,
      depth: false,
      stencil: false,
      premultipliedAlpha: true,
      preserveDrawingBuffer: false,
    }) as WebGL2RenderingContext | null
  } catch {
    return null
  }
  if (!gl) return null
  if (drawnBySoftware(gl)) {
    gl.getExtension('WEBGL_lose_context')?.loseContext()
    return null
  }

  const program = link(gl)
  const walkBuffer = gl.createBuffer()
  const geometryBuffer = gl.createBuffer()
  const colourBuffer = gl.createBuffer()
  const array = gl.createVertexArray()
  if (!program || !walkBuffer || !geometryBuffer || !colourBuffer || !array) {
    if (program) gl.deleteProgram(program)
    return null
  }

  const where = {
    resolution: gl.getUniformLocation(program, 'u_resolution'),
    scale: gl.getUniformLocation(program, 'u_scale'),
    offset: gl.getUniformLocation(program, 'u_offset'),
    half: gl.getUniformLocation(program, 'u_half'),
    core: gl.getUniformLocation(program, 'u_core'),
    dashOn: gl.getUniformLocation(program, 'u_dashOn'),
    dashPeriod: gl.getUniformLocation(program, 'u_dashPeriod'),
  }
  const attribute = (name: string) => gl.getAttribLocation(program, name)

  gl.bindVertexArray(array)
  gl.bindBuffer(gl.ARRAY_BUFFER, walkBuffer)
  gl.bufferData(gl.ARRAY_BUFFER, stripWalk(), gl.STATIC_DRAW)
  const walk = attribute('a_walk')
  gl.enableVertexAttribArray(walk)
  gl.vertexAttribPointer(walk, 2, gl.FLOAT, false, 0, 0)

  gl.bindBuffer(gl.ARRAY_BUFFER, geometryBuffer)
  const stride = GEOMETRY_STRIDE * 4
  const geometry: [string, number, number][] = [['a_from', 2, 0], ['a_control', 2, 8], ['a_to', 2, 16], ['a_dashed', 1, 24]]
  geometry.forEach(([name, size, offset]) => {
    const at = attribute(name)
    gl.enableVertexAttribArray(at)
    gl.vertexAttribPointer(at, size, gl.FLOAT, false, stride, offset)
    gl.vertexAttribDivisor(at, 1)
  })

  gl.bindBuffer(gl.ARRAY_BUFFER, colourBuffer)
  const colour = attribute('a_colour')
  gl.enableVertexAttribArray(colour)
  gl.vertexAttribPointer(colour, 4, gl.UNSIGNED_BYTE, true, 0, 0)
  gl.vertexAttribDivisor(colour, 1)
  gl.bindVertexArray(null)

  gl.disable(gl.DEPTH_TEST)
  gl.enable(gl.BLEND)
  // The shader writes premultiplied colour, so a faded hairline crossing a lit
  // one leaves it lit rather than washing it out.
  gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)

  let backingWidth = 0
  let backingHeight = 0
  // The geometry is rebuilt only when the layout itself moved: a pan, a zoom,
  // a hover and a hidden shelf all leave every curve exactly where it was.
  let laidOut: readonly MapNode[] | null = null
  let drawn: readonly MapEdge[] | null = null
  let instances = 0
  let paint: Uint8Array | null = null
  // Which link of the frame each instance came from. A link whose ends are not
  // both on the map is not drawn, so the colours the map handed down are read
  // through this rather than straight across.
  let kept: Int32Array = new Int32Array(0)
  let gathered: Uint8Array = new Uint8Array(0)

  const rebuild = (frame: EdgeFrame) => {
    const at = new Map<string, MapNode>(frame.nodes.map(node => [node.path, node]))
    const geometry = new Float32Array(frame.edges.length * GEOMETRY_STRIDE)
    const from_ = new Int32Array(frame.edges.length)
    let count = 0
    frame.edges.forEach((edge, position) => {
      const from = at.get(edge.from)
      const to = at.get(edge.to)
      if (!from || !to) return
      const control = curveControl(from, to)
      from_[count] = position
      const base = count * GEOMETRY_STRIDE
      geometry[base] = from.x
      geometry[base + 1] = from.y
      geometry[base + 2] = control.x
      geometry[base + 3] = control.y
      geometry[base + 4] = to.x
      geometry[base + 5] = to.y
      geometry[base + 6] = edge.tag ? 1 : 0
      count++
    })
    instances = count
    kept = from_.subarray(0, count)
    gathered = new Uint8Array(count * 4)
    paint = null
    gl.bindBuffer(gl.ARRAY_BUFFER, geometryBuffer)
    gl.bufferData(gl.ARRAY_BUFFER, geometry.subarray(0, count * GEOMETRY_STRIDE), gl.STATIC_DRAW)
  }

  return {
    draw(frame: EdgeFrame) {
      const width = Math.round(frame.width * frame.ratio)
      const height = Math.round(frame.height * frame.ratio)
      if (width !== backingWidth || height !== backingHeight) {
        canvas.width = width
        canvas.height = height
        backingWidth = width
        backingHeight = height
      }
      canvas.style.width = `${frame.width}px`
      canvas.style.height = `${frame.height}px`
      gl.viewport(0, 0, width, height)
      gl.clearColor(0, 0, 0, 0)
      gl.clear(gl.COLOR_BUFFER_BIT)

      if (laidOut !== frame.nodes || drawn !== frame.edges) {
        laidOut = frame.nodes
        drawn = frame.edges
        rebuild(frame)
      }
      if (instances === 0) return

      gl.bindVertexArray(array)
      if (paint !== frame.colours) {
        paint = frame.colours
        let sent = frame.colours
        if (instances !== frame.edges.length) {
          for (let index = 0; index < instances; index++) {
            gathered.set(frame.colours.subarray(kept[index] * 4, kept[index] * 4 + 4), index * 4)
          }
          sent = gathered
        }
        gl.bindBuffer(gl.ARRAY_BUFFER, colourBuffer)
        gl.bufferData(gl.ARRAY_BUFFER, sent.subarray(0, instances * 4), gl.DYNAMIC_DRAW)
      }
      gl.useProgram(program)
      gl.uniform2f(where.resolution, frame.width, frame.height)
      gl.uniform1f(where.scale, frame.scale)
      gl.uniform2f(where.offset, frame.offsetX, frame.offsetY)
      gl.uniform1f(where.half, HALF_WIDTH)
      gl.uniform1f(where.core, CORE)
      gl.uniform1f(where.dashOn, DASH_ON)
      gl.uniform1f(where.dashPeriod, DASH_PERIOD)
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, (CURVE_STEPS + 1) * 2, instances)
      gl.bindVertexArray(null)
    },
    destroy() {
      gl.deleteBuffer(walkBuffer)
      gl.deleteBuffer(geometryBuffer)
      gl.deleteBuffer(colourBuffer)
      gl.deleteVertexArray(array)
      gl.deleteProgram(program)
      // A browser holds only a handful of contexts at once, and the map is made
      // again every time the tab is opened.
      gl.getExtension('WEBGL_lose_context')?.loseContext()
    },
  }
}
