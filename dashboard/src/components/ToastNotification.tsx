import { useEffect, useState } from 'react'
import { useToast, Toast, ToastType } from '../context/ToastContext'

const TOAST_DURATION = 4000 // 4 seconds
const ANIMATION_DURATION = 300 // matches CSS transition

// Icons for each toast type
const TOAST_ICONS: Record<ToastType, string> = {
  success: '✓',
  info: 'ℹ',
  warning: '⚠',
  error: '✕',
}

interface ToastItemProps {
  toast: Toast
  onRemove: (id: number) => void
}

function ToastItem({ toast, onRemove }: ToastItemProps) {
  const [isExiting, setIsExiting] = useState(false)
  const [progress, setProgress] = useState(100)

  useEffect(() => {
    // Start the progress bar countdown
    const startTime = Date.now()
    const progressInterval = setInterval(() => {
      const elapsed = Date.now() - startTime
      const remaining = Math.max(0, 100 - (elapsed / TOAST_DURATION) * 100)
      setProgress(remaining)
    }, 50)

    // Auto-dismiss timer
    const dismissTimer = setTimeout(() => {
      setIsExiting(true)
      // Wait for exit animation before removing
      setTimeout(() => onRemove(toast.id), ANIMATION_DURATION)
    }, TOAST_DURATION)

    return () => {
      clearInterval(progressInterval)
      clearTimeout(dismissTimer)
    }
  }, [toast.id, onRemove])

  const handleClose = () => {
    setIsExiting(true)
    setTimeout(() => onRemove(toast.id), ANIMATION_DURATION)
  }

  return (
    <div
      className={`toast-item toast-${toast.type} ${isExiting ? 'toast-exit' : 'toast-enter'}`}
      role="alert"
      aria-live="polite"
    >
      <div className="toast-icon">{TOAST_ICONS[toast.type]}</div>
      <div className="toast-content">
        <span className="toast-message">{toast.message}</span>
      </div>
      <button
        className="toast-close"
        onClick={handleClose}
        aria-label="Dismiss notification"
      >
        ✕
      </button>
      <div className="toast-progress">
        <div
          className="toast-progress-bar"
          style={{ width: `${progress}%` }}
        />
      </div>
    </div>
  )
}

export function ToastContainer() {
  const { toasts, removeToast } = useToast()

  if (toasts.length === 0) return null

  return (
    <div className="toast-container" aria-label="Notifications">
      {toasts.map(toast => (
        <ToastItem key={toast.id} toast={toast} onRemove={removeToast} />
      ))}
    </div>
  )
}

export default ToastContainer
