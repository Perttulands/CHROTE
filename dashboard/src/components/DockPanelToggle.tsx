interface DockPanelToggleProps {
  label: string
  collapsed: boolean
  onToggle: () => void
}

function DockPanelToggle({ label, collapsed, onToggle }: DockPanelToggleProps) {
  return (
    <button
      className="toggle-btn dock-toggle-btn"
      type="button"
      aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${label} panel`}
      title={`${collapsed ? 'Expand' : 'Collapse'} ${label}`}
      onClick={onToggle}
    >
      <span className="dock-toggle-label">{label}</span>
      <span className="dock-toggle-chevron" aria-hidden="true">{collapsed ? '>>' : '<<'}</span>
    </button>
  )
}

export default DockPanelToggle
