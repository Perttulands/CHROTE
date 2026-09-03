/**
 * The plain editor.
 *
 * CHROTE has one editor and every surface that lets the operator change a file
 * uses it: the Files panel, and after them the Library and the instructions
 * tab. It is a textarea with a line-number gutter — no modes, no syntax
 * machinery, no autosave — because the edit it exists for is a small manual one
 * the operator makes while an agent is running.
 *
 * Three keys belong to the editor itself. Tab inserts two spaces, so indenting
 * does not tab out of the field. Ctrl+S saves, because that is the key every
 * operator already presses and the browser's own use of it is worth nothing
 * here. Escape asks: the editor does not decide what discarding means, it tells
 * the surface, and the surface asks with the control that does it.
 */

import { useRef } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'

export interface EditorProps {
  /** The text being edited. The surface owns it. */
  value: string
  onChange: (next: string) => void
  /** Ctrl+S, and whatever the surface's Save control does. */
  onSave: () => void
  /** Escape. The surface asks the question; the editor only reports the key. */
  onCancel: () => void
  /** What the field is, for a reader who cannot see it. */
  label: string
  autoFocus?: boolean
}

/** Two spaces, because that is what CHROTE's own sources are written in. */
export const EDITOR_INDENT = '  '

function lineNumbers(value: string): string {
  const count = value.split('\n').length
  const numbers = new Array<string>(count)
  for (let line = 1; line <= count; line += 1) numbers[line - 1] = String(line)
  return numbers.join('\n')
}

function Editor({ value, onChange, onSave, onCancel, label, autoFocus = false }: EditorProps) {
  const gutterRef = useRef<HTMLPreElement>(null)

  const handleKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Tab' && !event.altKey && !event.ctrlKey && !event.metaKey && !event.shiftKey) {
      event.preventDefault()
      const field = event.currentTarget
      const { selectionStart, selectionEnd } = field
      const next = `${value.slice(0, selectionStart)}${EDITOR_INDENT}${value.slice(selectionEnd)}`
      const caret = selectionStart + EDITOR_INDENT.length
      onChange(next)
      requestAnimationFrame(() => field.setSelectionRange(caret, caret))
      return
    }
    if (event.key.toLowerCase() === 's' && (event.ctrlKey || event.metaKey) && !event.altKey) {
      event.preventDefault()
      onSave()
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onCancel()
    }
  }

  return (
    <div className="chrote-editor">
      <pre className="chrote-editor-gutter" ref={gutterRef} aria-hidden="true">{lineNumbers(value)}</pre>
      <textarea
        className="chrote-editor-input"
        aria-label={label}
        value={value}
        wrap="off"
        spellCheck={false}
        autoComplete="off"
        autoFocus={autoFocus}
        onChange={event => onChange(event.target.value)}
        onKeyDown={handleKeyDown}
        onScroll={event => {
          if (gutterRef.current) gutterRef.current.scrollTop = event.currentTarget.scrollTop
        }}
      />
    </div>
  )
}

export default Editor
import './Editor.css'
