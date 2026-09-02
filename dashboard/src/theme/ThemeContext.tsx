import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { DEFAULT_THEME, applyTheme, loadTheme, type Theme } from './theme'

// The theme is read once, at startup, and never changes for the life of the
// page. The context exists so components can read the palette itself — the
// identity colours and the terminal object — not so they can change it.

export const ThemeContext = createContext<Theme>(DEFAULT_THEME)

/** The active theme. Outside a provider this is the built-in default. */
export function useTheme(): Theme {
  return useContext(ThemeContext)
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(DEFAULT_THEME)

  useEffect(() => {
    let current = true
    void loadTheme().then(loaded => {
      if (current) setTheme(loaded)
    })
    return () => { current = false }
  }, [])

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  return <ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>
}
