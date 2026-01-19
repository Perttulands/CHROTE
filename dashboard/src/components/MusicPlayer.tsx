import { useRef, useEffect, useState } from 'react'
import { useSession } from '../context/SessionContext'

const TRACKS = [
  { file: '/music/Convoy_Run.mp3', name: 'Convoy Run' },
  { file: '/music/Deacons_revenge.mp3', name: "Deacon's Revenge" },
  { file: '/music/Design_phase.mp3', name: 'Design Phase' },
  { file: '/music/March_of_the_Polecats.mp3', name: 'March of the Polecats' },
  { file: '/music/Mayors_introspection.mp3', name: "Mayor's Introspection" },
  { file: '/music/MergePush.mp3', name: 'Merge Push' },
  { file: '/music/Polecat_Danceparty.mp3', name: 'Polecat Danceparty' },
  { file: '/music/The_idle_Polecat.mp3', name: 'The Idle Polecat' },
  { file: '/music/Vibes_at_the_hq.mp3', name: 'Vibes at the HQ' },
  { file: '/music/Who_ate_my_PRD.mp3', name: 'Who Ate My PRD?' },
]

function MusicPlayer() {
  const audioRef = useRef<HTMLAudioElement>(null)
  const { settings, updateSettings } = useSession()
  const [track, setTrack] = useState(0)
  const [showTracks, setShowTracks] = useState(false)

  // Audio control effect
  useEffect(() => {
    const audio = audioRef.current
    if (!audio) return

    audio.volume = Math.max(0, Math.min(1, settings.musicVolume ?? 0.5))

    if (settings.musicEnabled) {
      audio.play().catch(e => {
        console.warn('Audio playback failed:', e.message)
        updateSettings({ musicEnabled: false })
      })
    } else {
      audio.pause()
    }
  }, [settings.musicEnabled, settings.musicVolume, updateSettings])

  // Track change effect
  useEffect(() => {
    const audio = audioRef.current
    if (!audio) return
    audio.src = TRACKS[track].file
    if (settings.musicEnabled) audio.play().catch(e => console.warn('Track change play failed:', e.message))
  }, [track, settings.musicEnabled])

  const next = () => setTrack(t => (t + 1) % TRACKS.length)
  const prev = () => setTrack(t => (t - 1 + TRACKS.length) % TRACKS.length)

  const selectTrack = (i: number) => {
    setTrack(i)
    setShowTracks(false)
    if (!settings.musicEnabled) updateSettings({ musicEnabled: true })
  }

  return (
    <div className="music-player">
      <audio ref={audioRef} onEnded={next} />

      <div className="music-controls">
        <button className="music-btn" onClick={prev} title="Previous">|◀</button>

        <button
          className={`music-btn music-toggle ${settings.musicEnabled ? 'playing' : ''}`}
          onClick={() => updateSettings({ musicEnabled: !settings.musicEnabled })}
          title={settings.musicEnabled ? 'Pause' : 'Play'}
        >
          {settings.musicEnabled ? '⏸' : '▶'}
        </button>

        <button className="music-btn" onClick={next} title="Next">▶|</button>

        {/* Track selector - click to toggle */}
        <div className="music-dropdown-wrap">
          <button
            className="music-btn music-track-btn"
            onClick={() => setShowTracks(!showTracks)}
          >
            {TRACKS[track].name} <span className="track-num">{track + 1}/{TRACKS.length}</span>
          </button>
          {showTracks && (
            <div className="music-dropdown">
              {TRACKS.map((t, i) => (
                <button
                  key={i}
                  className={`music-dropdown-item ${i === track ? 'active' : ''}`}
                  onClick={() => selectTrack(i)}
                >
                  {i + 1}. {t.name} {i === track && settings.musicEnabled && '▶'}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Inline volume slider */}
        <div className="music-volume-inline">
          <span className="vol-icon">{settings.musicVolume === 0 ? '🔇' : settings.musicVolume < 0.5 ? '🔉' : '🔊'}</span>
          <input
            type="range"
            min="0"
            max="1"
            step="0.1"
            value={settings.musicVolume}
            onChange={e => updateSettings({ musicVolume: parseFloat(e.target.value) })}
            className="volume-slider-inline"
            title={`Volume: ${Math.round(settings.musicVolume * 100)}%`}
          />
        </div>
      </div>

      {/* Click outside to close dropdown */}
      {showTracks && <div className="music-backdrop" onClick={() => setShowTracks(false)} />}
    </div>
  )
}

export default MusicPlayer
