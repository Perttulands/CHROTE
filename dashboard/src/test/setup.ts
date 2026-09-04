import '@testing-library/jest-dom'

// jsdom draws nothing: its canvas has no context unless a native package is
// installed, and asking for one prints a page of noise per test. The map is
// built to draw only when it has a surface, so the answer given here is the
// honest one — there is none — given quietly.
HTMLCanvasElement.prototype.getContext = (() => null) as unknown as typeof HTMLCanvasElement.prototype.getContext

// jsdom ships no matchMedia. xterm.js queries it for device pixel ratio the
// moment a terminal opens, so without this every terminal test throws.
if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    media: query,
    matches: false,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia
}
