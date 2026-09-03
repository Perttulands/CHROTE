/**
 * A path drawn so the file name survives.
 *
 * Panels are narrow and paths are long, so something has to give. What must
 * not give is the last segment: a list of five paths ending in `…/agent-a5…`
 * tells the operator nothing, while one ending in `journeys.md` tells him
 * everything. The directory is therefore the part that shortens, and it
 * shortens in CSS rather than by measurement, so it stays right at every panel
 * width without a resize observer to keep it honest.
 */

export interface PanelPathProps {
  path: string
  className?: string
}

function PanelPath({ path, className }: PanelPathProps) {
  const cut = path.lastIndexOf('/')
  const head = cut >= 0 ? path.slice(0, cut + 1) : ''
  const tail = cut >= 0 ? path.slice(cut + 1) : path

  return (
    <span className={`panel-path ${className || ''}`} title={path}>
      <span className="panel-path-head">{head}</span>
      <span className="panel-path-tail">{tail}</span>
    </span>
  )
}

export default PanelPath
