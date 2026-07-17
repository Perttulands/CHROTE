export type FormationsInteractionKind = 'pan' | 'node' | 'wire' | 'gate' | 'lane' | 'staff'

export type InteractionPointer = {
  pointerId: number
  clientX: number
  clientY: number
  target: EventTarget | null
}

export type FormationsInteraction = {
  kind: FormationsInteractionKind
  pointerId: number
  project: (pointer: InteractionPointer) => void
  finalize: (pointer: InteractionPointer) => void
  cancel: () => void
}

export type FormationsInteractionOwner = {
  projection: () => Pick<FormationsInteraction, 'kind' | 'pointerId'> | null
  begin: (interaction: FormationsInteraction) => void
  project: (pointer: InteractionPointer) => boolean
  finalize: (pointer: InteractionPointer) => boolean
  cancel: (pointerId?: number) => boolean
}

export function createFormationsInteractionOwner(): FormationsInteractionOwner {
  let active: FormationsInteraction | null = null

  const settle = (pointerId: number | undefined, finalizeWith?: InteractionPointer) => {
    if (!active || (pointerId !== undefined && active.pointerId !== pointerId)) return false
    const interaction = active
    active = null
    interaction.cancel()
    if (finalizeWith) interaction.finalize(finalizeWith)
    return true
  }

  return {
    projection: () => active ? { kind: active.kind, pointerId: active.pointerId } : null,
    begin: interaction => {
      settle(undefined)
      active = interaction
    },
    project: pointer => {
      if (!active || active.pointerId !== pointer.pointerId) return false
      active.project(pointer)
      return true
    },
    finalize: pointer => settle(pointer.pointerId, pointer),
    cancel: pointerId => settle(pointerId),
  }
}
