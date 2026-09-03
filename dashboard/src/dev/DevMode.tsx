/**
 * Dev mode: point at what is wrong and hand it to an agent.
 *
 * The operator sees something the dashboard does badly and wants to say so
 * without first working out which file it lives in. Dev mode turns the whole
 * page into a target: a highlight follows the pointer, a corner label names the
 * component under it and the file it is written in, and a click opens the Send
 * drawer with that line already written. He types the complaint and sends it.
 *
 * While it is on, a click annotates instead of pressing: the press is caught in
 * the capture phase before the app can see it, so pointing at the Kill button
 * does not kill anything. Escape ends it, and so does the chord.
 *
 * There is nothing to poll and nothing to reload into: the highlight is driven
 * by pointer events alone, and dev mode is a fact about this browsing session
 * that no reload carries over.
 *
 * Chrome owns Alt+D, so the direct chord is Alt+Shift+D; the leader's keys
 * panel reaches it too, as it reaches every chord.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { registerChords } from '../keys/chords'
import { identifyElement, labelFor, placeOf, referenceFor } from './identify'
import './DevMode.css'

interface Highlight {
  label: string
  top: number
  left: number
  width: number
  height: number
}

function highlightOf(node: unknown): Highlight | null {
  const identity = identifyElement(node)
  if (!identity) return null
  const box = identity.element.getBoundingClientRect()
  return { label: labelFor(identity), top: box.top, left: box.left, width: box.width, height: box.height }
}

export default function DevMode({ activeTab }: { activeTab: string }) {
  const { openSendToSession } = useSession()
  const { announce } = useStatus()
  const [on, setOn] = useState(false)
  const [seen, setSeen] = useState<Highlight | null>(null)

  // The chord is registered once and reads the live state through refs, so a
  // re-render never churns the registry and the keys panel never flickers.
  const onRef = useRef(false)
  const apply = useCallback((next: boolean) => {
    if (onRef.current === next) return
    onRef.current = next
    setOn(next)
    if (!next) setSeen(null)
    announce(next ? 'Dev mode on' : 'Dev mode off', 'info')
  }, [announce])
  const applyRef = useRef(apply)
  applyRef.current = apply

  useEffect(() => registerChords([{
    id: 'keys.dev',
    key: 'd',
    label: 'Dev mode',
    direct: { alt: true, shift: true, key: 'd' },
    scope: 'global',
    run: () => applyRef.current(!onRef.current),
  }]), [])

  useEffect(() => {
    if (!on) return

    const look = (event: PointerEvent) => setSeen(highlightOf(event.target))

    // Only the propagation is stopped on the press: cancelling pointerdown
    // would cancel the click the annotation is waiting for. The app never sees
    // it either way, which is what stops the button from being pressed.
    const hold = (event: Event) => event.stopPropagation()

    const annotate = (event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
      const identity = identifyElement(event.target)
      apply(false)
      if (!identity) return
      openSendToSession({
        reference: referenceFor(identity, placeOf(identity.element, activeTab)),
        // A complaint about CHROTE's own surface belongs to whoever works on
        // CHROTE, so the picker offers that agent by name with its harness
        // already chosen. The folder stays the launcher's: nothing the browser
        // is told names the repository's place on the host.
        launch: { label: 'New agent in CHROTE', harness: 'claude-code' },
      })
    }

    const leave = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      event.stopPropagation()
      apply(false)
    }

    document.addEventListener('pointermove', look, true)
    document.addEventListener('pointerdown', hold, true)
    document.addEventListener('mousedown', hold, true)
    document.addEventListener('mouseup', hold, true)
    document.addEventListener('click', annotate, true)
    document.addEventListener('keydown', leave, true)
    return () => {
      document.removeEventListener('pointermove', look, true)
      document.removeEventListener('pointerdown', hold, true)
      document.removeEventListener('mousedown', hold, true)
      document.removeEventListener('mouseup', hold, true)
      document.removeEventListener('click', annotate, true)
      document.removeEventListener('keydown', leave, true)
    }
  }, [on, activeTab, apply, openSendToSession])

  if (!on) return null

  return (
    <>
      {seen && (
        <div
          className="dev-mode-outline"
          aria-hidden="true"
          style={{ top: seen.top, left: seen.left, width: seen.width, height: seen.height }}
        />
      )}
      <div className="dev-mode-label">{seen?.label ?? 'Dev mode · point at something'}</div>
    </>
  )
}
