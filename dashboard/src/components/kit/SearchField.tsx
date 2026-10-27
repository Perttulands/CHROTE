/**
 * The kit's search field, the one way a view asks for words to find by.
 *
 * A 28px box over a hairline that takes the accent while it has the keyboard,
 * the placeholder saying what it searches, and room at its right for a quiet
 * hint: the chord that reaches it, or how many it found. What a key does beyond
 * typing (Down into the results, Enter to open the first) belongs to the view,
 * which knows its results, so it comes in through `onKeyDown`.
 */

import type { KeyboardEvent, ReactNode, Ref } from 'react'
import './kit.css'

interface SearchFieldProps {
  value: string
  onChange: (value: string) => void
  /** What the field is called: "Search Beads". */
  label: string
  /** What it searches, as the operator reads it; the label when absent. */
  placeholder?: string
  onKeyDown?: (event: KeyboardEvent<HTMLInputElement>) => void
  inputRef?: Ref<HTMLInputElement>
  autoFocus?: boolean
  title?: string
  className?: string
  /** Quiet text at the field's right, such as a chord or a count. */
  hint?: ReactNode
}

export default function SearchField({ value, onChange, label, placeholder, onKeyDown, inputRef, autoFocus, title, className, hint }: SearchFieldProps) {
  return (
    <label className={`kit-search${className ? ` ${className}` : ''}`} title={title}>
      <input
        ref={inputRef}
        type="search"
        value={value}
        placeholder={placeholder ?? `${label}…`}
        aria-label={label}
        autoComplete="off"
        spellCheck={false}
        autoFocus={autoFocus}
        onChange={event => onChange(event.target.value)}
        onKeyDown={onKeyDown}
      />
      {hint !== undefined && <span className="kit-search-hint">{hint}</span>}
    </label>
  )
}
