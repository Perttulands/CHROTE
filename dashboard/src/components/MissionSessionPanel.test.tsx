import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import MissionSessionPanel, { formationsTerminalUrl, missionSessionName } from './MissionSessionPanel'
import type { TmuxSession } from '../types'

/* The open-board session side-panel. It renders the FORMATIONS socket sessions
   (owned by MissionsView) and attaches them by pointing an iframe at
   /terminal-formations/?arg=<name> — never the cockpit's /terminal/ surface, so
   the Terminal tabs are unaffected. */

function session(name: string): TmuxSession {
  return { name, windows: 1, attached: false, group: 'mission' }
}

function baseProps() {
  return {
    sessions: [] as TmuxSession[],
    error: '',
    loading: false,
    personaStems: [] as string[],
    selectedSession: null as string | null,
    onSelectSession: vi.fn(),
    onRefresh: vi.fn(),
    onSpawn: vi.fn().mockResolvedValue(undefined),
  }
}

describe('MissionSessionPanel', () => {
  afterEach(() => cleanup())

  it('derives the formations session name and attach URL for a persona stem', () => {
    expect(missionSessionName('scout')).toBe('mission-scout')
    // The attach URL points at the second ttyd proxy, not the cockpit's.
    expect(formationsTerminalUrl('mission-scout')).toContain('/terminal-formations/?arg=mission-scout')
    expect(formationsTerminalUrl('mission-scout')).not.toContain('/terminal/?arg=')
  })

  it('lists the formations sessions as tabs', () => {
    render(<MissionSessionPanel {...baseProps()} sessions={[session('mission-scout'), session('mission-writer')]} />)
    expect(screen.getByRole('button', { name: 'mission-scout' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'mission-writer' })).toBeInTheDocument()
  })

  it('attaches the selected session via the /terminal-formations/ iframe', () => {
    render(<MissionSessionPanel {...baseProps()} sessions={[session('mission-scout')]} selectedSession="mission-scout" />)
    const iframe = screen.getByTitle('Mission agent — mission-scout') as HTMLIFrameElement
    expect(iframe.src).toContain('/terminal-formations/?arg=mission-scout')
    expect(iframe.src).not.toContain('/terminal/?arg=')
  })

  it('selects a session when its tab is clicked', () => {
    const onSelectSession = vi.fn()
    render(<MissionSessionPanel {...baseProps()} sessions={[session('mission-scout')]} onSelectSession={onSelectSession} />)
    fireEvent.click(screen.getByRole('button', { name: 'mission-scout' }))
    expect(onSelectSession).toHaveBeenCalledWith('mission-scout')
  })

  it('offers to spawn mission-<stem> for an assigned persona with no live session', async () => {
    const onSpawn = vi.fn().mockResolvedValue(undefined)
    render(<MissionSessionPanel {...baseProps()} personaStems={['scout']} sessions={[]} onSpawn={onSpawn} />)
    fireEvent.click(screen.getByRole('button', { name: /Spawn mission-scout/ }))
    await waitFor(() => expect(onSpawn).toHaveBeenCalledWith('scout'))
  })

  it('does not offer a spawn button when the persona already has a live session', () => {
    render(<MissionSessionPanel {...baseProps()} personaStems={['scout']} sessions={[session('mission-scout')]} />)
    expect(screen.queryByRole('button', { name: /Spawn mission-scout/ })).toBeNull()
  })

  it('surfaces a session-list error clearly instead of a silent empty list', () => {
    render(<MissionSessionPanel {...baseProps()} error="no formations tmux server running" />)
    expect(screen.getByRole('alert')).toHaveTextContent('no formations tmux server running')
  })
})
