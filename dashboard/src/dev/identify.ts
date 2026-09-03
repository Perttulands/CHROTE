/**
 * Naming what the operator is pointing at.
 *
 * A complaint about the dashboard is worth nothing to an agent until it names
 * the thing complained about. The browser can only say `div.session-item`; the
 * agent needs the component, the file it is written in, and the identity the
 * surface was given. All three come from what is already in the page.
 *
 * The component comes from React's own bookkeeping: every DOM node React made
 * carries a `__reactFiber$…` property, and walking that fiber's parents finds
 * the nearest function with a component's name. The file comes from the map
 * built over the source at build time, because nothing in a bundle remembers
 * where a function was written. The identity comes from `data-ui`, which a
 * person put there by hand on the surfaces worth naming.
 *
 * `data-ui` wins over the component, and it should: a component gets renamed
 * and split, and the operator's "the tile header" does not. Both are reported
 * when both exist, because they answer different halves of the question — what
 * this is, and where in the source it is written.
 */

import componentMap from '../generated/componentMap.json'

const FILES = componentMap as Record<string, string | undefined>

/** How much of an element's own words the reference carries. */
const TEXT_LIMIT = 80

/** How deep a wrapper chain (memo of forwardRef of …) is worth unwrapping. */
const WRAPPER_DEPTH = 4

/** What a role is when the element does not say: the ones with an obvious one. */
const IMPLICIT_ROLES: Record<string, string> = {
  a: 'link',
  button: 'button',
  input: 'input',
  textarea: 'textbox',
  select: 'listbox',
  img: 'image',
}

/** Only what this file reads of a React fiber. */
interface Fiber {
  return: Fiber | null
  child: Fiber | null
  type: unknown
  stateNode: unknown
}

export interface UiIdentity {
  /** The nearest identified surface: what the outline goes round. */
  element: HTMLElement
  /** What the words describe: the pointed element, or the nearest one that speaks. */
  subject: HTMLElement
  /** The nearest React component's name, or null outside React's tree. */
  component: string | null
  /** Where that component is written, when the map knows. */
  file: string | null
  /** The hand-given identity, when the surface has one. */
  uiId: string | null
  role: string
  text: string
}

/** Where in the dashboard the element sits, as the operator would say it. */
export interface UiPlace {
  tab: string
  /** The terminal window's number, when the element is inside one. */
  window: string | null
}

function fiberOf(node: Element): Fiber | null {
  for (const key of Object.keys(node)) {
    if (key.startsWith('__reactFiber$')) return (node as unknown as Record<string, Fiber>)[key]
  }
  return null
}

/**
 * The component name a fiber's type carries. A lower-case name is a host
 * element ('div'), and an unnamed function is a minifier that was not told to
 * keep names — both are answered with null rather than with a lie.
 */
function componentName(type: unknown, depth = 0): string | null {
  if (depth > WRAPPER_DEPTH) return null
  if (typeof type === 'function') {
    const declared = type as { displayName?: unknown; name?: unknown }
    const named = typeof declared.displayName === 'string'
      ? declared.displayName
      : typeof declared.name === 'string' ? declared.name : ''
    return /^[A-Z]/.test(named) ? named : null
  }
  if (typeof type === 'object' && type !== null) {
    const wrapper = type as { displayName?: unknown; render?: unknown; type?: unknown }
    if (typeof wrapper.displayName === 'string' && /^[A-Z]/.test(wrapper.displayName)) return wrapper.displayName
    return componentName(wrapper.render, depth + 1) ?? componentName(wrapper.type, depth + 1)
  }
  return null
}

/** The first DOM node a fiber rendered: where that component starts on screen. */
function hostNodeOf(fiber: Fiber): Element | null {
  let current = fiber.child
  while (current) {
    if (current.stateNode instanceof Element) return current.stateNode
    current = current.child
  }
  return null
}

/**
 * The nearest component of THIS dashboard, and only failing that the nearest
 * component of any kind. An icon from a library is a component too, and naming
 * it would send an agent into node_modules for a complaint about a tile header;
 * the map, built over `dashboard/src`, is what tells the two apart.
 */
function nearestComponent(node: Element): { name: string; host: Element | null } | null {
  let fiber = fiberOf(node)
  let foreign: { name: string; host: Element | null } | null = null
  while (fiber) {
    const name = componentName(fiber.type)
    if (name !== null) {
      if (FILES[name] !== undefined) return { name, host: hostNodeOf(fiber) }
      foreign ??= { name, host: hostNodeOf(fiber) }
    }
    fiber = fiber.return
  }
  return foreign
}

/**
 * What the pointer is really over, in words. An icon says nothing about itself,
 * so the button holding it speaks for it; the search stops at the surface,
 * which always has words of its own.
 */
function speakingNode(node: Element, limit: Element): Element {
  let current: Element | null = node
  while (current) {
    if (textOf(current) !== '') return current
    if (current === limit) break
    current = current.parentElement
  }
  return node
}

export function roleOf(element: Element): string {
  const explicit = element.getAttribute('role')?.trim()
  if (explicit) return explicit
  const tag = element.tagName.toLowerCase()
  return IMPLICIT_ROLES[tag] ?? tag
}

export function textOf(element: Element): string {
  const words = (element.textContent ?? '').replace(/\s+/g, ' ').trim()
  const said = words || element.getAttribute('aria-label')?.trim() || element.getAttribute('title')?.trim() || ''
  return said.length > TEXT_LIMIT ? `${said.slice(0, TEXT_LIMIT - 1).trimEnd()}…` : said
}

/**
 * What the pointer is over: the nearest named thing at or above the node, and
 * everything an agent needs to find it in the source.
 */
export function identifyElement(node: unknown): UiIdentity | null {
  if (!(node instanceof Element)) return null
  const component = nearestComponent(node)
  // A component that rendered a fragment can have a first host node that is not
  // an ancestor of what was pointed at; then the node speaks for itself.
  const owned = component?.host && component.host.contains(node) ? component.host : null
  const marked = node.closest('[data-ui]')
  // Both are ancestors of the same node, so one contains the other, and the
  // one further in is the one the operator was actually aiming at. The identity
  // is reported either way: a named surface holding an unnamed component still
  // says which surface it was.
  const element = (marked && owned
    ? (owned.contains(marked) ? marked : owned)
    : marked ?? owned ?? node) as HTMLElement
  // The outline snaps to the surface, but the words are the pointed thing's:
  // "the tile header" says where, and "button 'Send to main'" says what.
  const subject = speakingNode(node, element)
  return {
    element,
    subject: subject as HTMLElement,
    component: component?.name ?? null,
    file: component ? FILES[component.name] ?? null : null,
    uiId: marked?.getAttribute('data-ui') ?? null,
    role: roleOf(subject),
    text: textOf(subject),
  }
}

/** Which tab, and which terminal window, the element sits in. */
export function placeOf(element: Element, tab: string): UiPlace {
  return { tab, window: element.closest('[data-window]')?.getAttribute('data-window') ?? null }
}

/** What the corner label says: the component, its file, and its identity. */
export function labelFor(identity: UiIdentity): string {
  const named = identity.component ?? identity.element.tagName.toLowerCase()
  const parts = [named, identity.file ?? 'file unknown']
  if (identity.uiId) parts.push(identity.uiId)
  return parts.join(' · ')
}

/**
 * The one line the agent receives: what it is, where it is written, where it
 * sits, and what it says.
 */
export function referenceFor(identity: UiIdentity, place: UiPlace): string {
  const named = identity.component ?? identity.subject.tagName.toLowerCase()
  const parts = [`component ${named}`]
  if (identity.file) parts.push(`(${identity.file})`)
  if (identity.uiId) parts.push(identity.uiId)
  parts.push(`in ${place.tab}${place.window ? ` window ${place.window}` : ''}`)
  return `${parts.join(' ')}: ${identity.role}${identity.text ? ` '${identity.text}'` : ''}`
}
