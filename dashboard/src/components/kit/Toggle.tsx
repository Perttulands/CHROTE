/**
 * The kit's on/off control, in place of the browser's checkbox.
 *
 * With words it is a whole row that toggles: the mark, the words and anything
 * beside them (a count, an age) all take the press, at least 28px high. Without
 * words it is the mark alone in a 28px square, for a row whose name already
 * does something else. Either way it is a checkbox to assistive technology.
 */

import type { ReactNode } from 'react'
import './kit.css'

interface ToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  /** The visible words; absent, `label` names the bare mark. */
  children?: ReactNode
  /** What the control is called when it has no words of its own. */
  label?: string
  /** Quiet text at the row's right, such as a count. */
  meta?: ReactNode
  disabled?: boolean
  className?: string
  title?: string
}

export default function Toggle({ checked, onChange, children, label, meta, disabled, className, title }: ToggleProps) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      title={title}
      disabled={disabled}
      className={`kit-toggle${children === undefined ? ' bare' : ''}${className ? ` ${className}` : ''}`}
      onClick={() => onChange(!checked)}
    >
      <span className="kit-toggle-mark" aria-hidden="true" />
      {children !== undefined && <span className="kit-toggle-words">{children}</span>}
      {meta !== undefined && <span className="kit-toggle-meta">{meta}</span>}
    </button>
  )
}
