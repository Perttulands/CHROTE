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
      data-collapsed={collapsed}
      aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${label} panel`}
      title={`${collapsed ? 'Expand' : 'Collapse'} ${label}`}
      onClick={onToggle}
    >
      <span className="dock-toggle-content">
        <span className="dock-toggle-label">{label}</span>
        <span className="dock-toggle-chevron" aria-hidden="true">{collapsed ? '>>' : '<<'}</span>
      </span>
    </button>
  )
}

export default DockPanelToggle
