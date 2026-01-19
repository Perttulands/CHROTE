import { useEffect } from 'react'

interface ErrorToastProps {
  message: string
  onDismiss: () => void
  autoDismissMs?: number
}

export function ErrorToast({ message, onDismiss, autoDismissMs = 5000 }: ErrorToastProps) {
  useEffect(() => {
    const timer = setTimeout(onDismiss, autoDismissMs)
    return () => clearTimeout(timer)
  }, [onDismiss, autoDismissMs])

  return (
    <div className="fb-error-toast" role="alert">
      <span className="fb-error-toast-icon">!</span>
      <span className="fb-error-toast-message">{message}</span>
      <button
        className="fb-error-toast-dismiss"
        onClick={onDismiss}
        aria-label="Dismiss error"
      >
        x
      </button>
    </div>
  )
}
