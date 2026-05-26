import { createContext, useContext, useState, useCallback, ReactNode, useMemo } from 'react'

export type ToastType = 'success' | 'info' | 'warning' | 'error'

export interface Toast {
  id: number
  message: string
  type: ToastType
  createdAt: number
}

interface ToastContextType {
  toasts: Toast[]
  addToast: (message: string, type: ToastType) => void
  removeToast: (id: number) => void
}

const ToastContext = createContext<ToastContextType | null>(null)

const MAX_TOASTS = 5

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const [nextId, setNextId] = useState(1)

  const addToast = useCallback((message: string, type: ToastType) => {
    const id = nextId
    setNextId(prev => prev + 1)

    const newToast: Toast = {
      id,
      message,
      type,
      createdAt: Date.now(),
    }

    setToasts(prev => {
      // Add new toast to the beginning (newest on top)
      const updated = [newToast, ...prev]
      // Keep only MAX_TOASTS (remove oldest if exceeded)
      if (updated.length > MAX_TOASTS) {
        return updated.slice(0, MAX_TOASTS)
      }
      return updated
    })
  }, [nextId])

  const removeToast = useCallback((id: number) => {
    setToasts(prev => prev.filter(toast => toast.id !== id))
  }, [])

  const contextValue = useMemo(() => ({
    toasts,
    addToast,
    removeToast,
  }), [toasts, addToast, removeToast])

  return (
    <ToastContext.Provider value={contextValue}>
      {children}
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextType {
  const context = useContext(ToastContext)
  if (!context) {
    throw new Error('useToast must be used within a ToastProvider')
  }
  return context
}
