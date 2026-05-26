// Project selector dropdown for Beads views

import type { BeadsProject } from './types'

interface ProjectSelectorProps {
  projects: BeadsProject[]
  selectedPath: string | null
  onSelect: (path: string) => void
  loading?: boolean
}

export default function ProjectSelector({ projects, selectedPath, onSelect, loading }: ProjectSelectorProps) {
  if (loading) {
    return (
      <div className="project-selector">
        <select disabled>
          <option>Loading projects...</option>
        </select>
      </div>
    )
  }

  if (projects.length === 0) {
    return (
      <div className="project-selector">
        <select disabled>
          <option>No projects with .beads found</option>
        </select>
      </div>
    )
  }

  return (
    <div className="project-selector">
      <label htmlFor="project-select">Project:</label>
      <select
        id="project-select"
        value={selectedPath || ''}
        onChange={(e) => onSelect(e.target.value)}
      >
        <option value="">Select a project</option>
        {projects.map(project => (
          <option key={project.path} value={project.path}>
            {project.name} ({project.path})
          </option>
        ))}
      </select>
    </div>
  )
}
