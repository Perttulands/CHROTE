/**
 * The kit's choice of one from a few, in place of the browser's select.
 *
 * At rest it is the current choice in words; pressed, it opens the product's
 * one menu flush under itself with the choices as rows and the current one
 * checked. Arrow keys, Enter and Escape behave as they do in every menu.
 */

import { useRef, useState } from 'react'
import Menu from '../Menu'
import './kit.css'

export interface ChoiceOption<T extends string> {
  value: T
  label: string
}

interface ChoiceProps<T extends string> {
  value: T
  options: ChoiceOption<T>[]
  onChange: (value: T) => void
  /** What is being chosen, for assistive technology and the menu. */
  label: string
  /** Shown when the value is none of the options. */
  placeholder?: string
  disabled?: boolean
  className?: string
  id?: string
  autoFocus?: boolean
}

export default function Choice<T extends string>({ value, options, onChange, label, placeholder, disabled, className, id, autoFocus }: ChoiceProps<T>) {
  const button = useRef<HTMLButtonElement>(null)
  const [at, setAt] = useState<{ x: number; y: number } | null>(null)
  const current = options.find(option => option.value === value)
  const open = () => {
    const box = button.current?.getBoundingClientRect()
    if (box) setAt({ x: box.left, y: box.bottom })
  }
  return (
    <>
      <button
        ref={button}
        id={id}
        type="button"
        autoFocus={autoFocus}
        className={`kit-choice${className ? ` ${className}` : ''}`}
        aria-haspopup="menu"
        aria-expanded={at !== null}
        aria-label={`${label}: ${current?.label ?? placeholder ?? 'none'}`}
        disabled={disabled}
        onClick={() => (at ? setAt(null) : open())}
        onKeyDown={event => {
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); open() }
        }}
      >
        <span className={current ? undefined : 'kit-choice-placeholder'}>{current?.label ?? placeholder ?? ''}</span>
      </button>
      {at && (
        <Menu
          at={at}
          label={label}
          estimatedSize={{ width: 220, height: options.length * 32 + 8 }}
          onClose={() => { setAt(null); button.current?.focus() }}
          groups={[{
            id: 'choices',
            rows: options.map(option => ({
              id: option.value || '(none)',
              label: option.label,
              checked: option.value === value,
              onSelect: () => onChange(option.value),
            })),
          }]}
        />
      )}
    </>
  )
}
