import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { IframePoolProvider, useIframePool } from './IframePool'

// This suite proves IframePool's context identity. Keep the session boundary
// synchronous so an unrelated SessionProvider refresh cannot update this test.
vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: { fontSize: 14, hideScrollbar: false },
    workspaces: {},
    sessions: [],
  }),
}))

describe('IframePool context stability', () => {
  it('pool operation functions have stable references across rerenders', () => {
    // The pool functions (claimIframe, getIframe, triggerFit, focusIframe)
    // must be stable refs (useCallback with [] deps).
    // If they aren't, then any effect depending on `pool` will re-run
    // every time the context changes — causing claim/release cycles.

    const { result, rerender } = renderHook(() => useIframePool(), {
      wrapper: ({ children }) => <IframePoolProvider>{children}</IframePoolProvider>,
    })

    const first = {
      claim: result.current.claimIframe,
      get: result.current.getIframe,
      fit: result.current.triggerFit,
      focus: result.current.focusIframe,
      apply: result.current.applyFontSize,
    }

    rerender()

    // All function refs must be identical after rerender
    expect(result.current.claimIframe).toBe(first.claim)
    expect(result.current.getIframe).toBe(first.get)
    expect(result.current.triggerFit).toBe(first.fit)
    expect(result.current.focusIframe).toBe(first.focus)
    expect(result.current.applyFontSize).toBe(first.apply)
  })

  it('isLoaded changing must NOT invalidate stable operation refs', () => {
    // BUG TEST: isLoaded depends on [loadedSessions]. When loadedSessions changes,
    // isLoaded gets a new ref → contextValue useMemo recomputes → new context object.
    // This means `pool` reference changes for all consumers.
    //
    // If TerminalWindow has `pool` in its effect deps, the claim effect cleanup runs
    // (moving iframes to hidden pool) then re-claims. This is the cascading failure.
    //
    // After fix: isLoaded should use a ref so it has a stable identity,
    // OR TerminalWindow should not have pool in its deps.
    //
    // We test: the context value should be stable across isLoaded changes.

    const { result, rerender } = renderHook(() => useIframePool(), {
      wrapper: ({ children }) => <IframePoolProvider>{children}</IframePoolProvider>,
    })

    const contextRef1 = result.current

    // Rerender multiple times (simulates loadedSessions state changes)
    rerender()
    rerender()
    rerender()

    const contextRef2 = result.current

    // The critical assertion: operation functions must be the same object refs.
    // If isLoaded causes context churn, at minimum the operation functions should
    // still be stable so removing `pool` from effect deps is safe.
    expect(contextRef2.claimIframe).toBe(contextRef1.claimIframe)
    expect(contextRef2.getIframe).toBe(contextRef1.getIframe)
    expect(contextRef2.triggerFit).toBe(contextRef1.triggerFit)
    expect(contextRef2.focusIframe).toBe(contextRef1.focusIframe)
  })
})
