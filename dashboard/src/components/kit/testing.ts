/** Test helpers that drive the kit as the operator does: by its words. */

import { fireEvent, screen } from '@testing-library/react'

const literal = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

/** The Choice called `label`, whatever it currently reads. */
export function choice(label: string): HTMLElement {
  return screen.getByRole('button', { name: new RegExp(`^${literal(label)}: `) })
}

/** What the Choice called `label` currently reads. */
export function choiceValue(label: string): string {
  return choice(label).textContent ?? ''
}

/** Open the Choice called `label` and pick the option reading `option`. */
export function choose(label: string, option: string) {
  fireEvent.click(choice(label))
  fireEvent.click(screen.getByRole('menuitem', { name: new RegExp(`^${literal(option)}\\b`) }))
}
