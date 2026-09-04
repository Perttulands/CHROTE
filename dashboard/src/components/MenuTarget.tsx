/**
 * An object with a menu.
 *
 * Wraps the one element that is the object — a row, a header — with the
 * gestures that open its menu, and draws the Menu when they do. The rows are
 * asked for only then, so a list of a thousand objects builds nothing until
 * the operator points at one. The element's own right-click and touch
 * handlers, if it had any, are replaced: the menu is what those gestures mean
 * on an object that has one.
 */

import { cloneElement } from 'react'
import type { ReactElement } from 'react'
import Menu, { type MenuGroup } from './Menu'
import { useContextMenu, type MenuAnchor, type MenuTriggerProps } from './useContextMenu'

interface MenuTargetProps {
  /** What the menu is for, for assistive technology. */
  label: string
  /** The rows, asked for when the menu opens, told where it opened. */
  groups: (at: MenuAnchor) => MenuGroup[]
  /** A selector for the parts of the object whose gestures belong to themselves. */
  ignore?: string
  children: ReactElement
}

export default function MenuTarget({ label, groups, ignore, children }: MenuTargetProps) {
  const { anchor, close, triggerProps } = useContextMenu({ ignore })
  return (
    <>
      {cloneElement(children as ReactElement<MenuTriggerProps>, triggerProps)}
      {anchor && <Menu at={anchor} label={label} groups={groups(anchor)} onClose={close} />}
    </>
  )
}
