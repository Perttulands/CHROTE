import '@testing-library/jest-dom'

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
