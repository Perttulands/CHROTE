import {
  WEEKDAYS,
  browserTimezone,
  describeSchedule,
  scheduleFromForm,
  type ScheduleForm,
  type ScheduleMode,
} from './scheduledSchedule'

export default function ScheduleEditor({ schedule, onChange }: { schedule: ScheduleForm; onChange: (schedule: ScheduleForm) => void }) {
  const modes: { mode: ScheduleMode; label: string }[] = [
    { mode: 'every', label: 'Every' },
    { mode: 'daily', label: 'Daily' },
    { mode: 'weekly', label: 'Weekly' },
    { mode: 'cron', label: 'Cron' },
  ]
  const localZone = browserTimezone()

  return (
    <div className="scheduled-when">
      <div className="scheduled-targets-head">
        <span className="scheduled-label">When</span>
        <span className="scheduled-muted">{describeSchedule(scheduleFromForm(schedule))} · {schedule.timezone}</span>
      </div>

      <div className="scheduled-mode-switch" role="group" aria-label="Schedule type">
        {modes.map(({ mode, label }) => (
          <button
            key={mode}
            type="button"
            className={`scheduled-mode ${schedule.mode === mode ? 'active' : ''}`}
            aria-pressed={schedule.mode === mode}
            onClick={() => onChange({ ...schedule, mode })}
          >
            {label}
          </button>
        ))}
      </div>

      <div className="scheduled-when-row">
        {schedule.mode === 'every' && (
          <>
            <label className="scheduled-field compact">
              <span>Run every</span>
              <input
                type="number"
                min="1"
                aria-label="Interval"
                value={schedule.everyValue}
                onChange={event => onChange({ ...schedule, everyValue: event.target.value })}
              />
            </label>
            <label className="scheduled-field compact">
              <span>Unit</span>
              <select
                aria-label="Interval unit"
                value={schedule.everyUnit}
                onChange={event => onChange({ ...schedule, everyUnit: event.target.value as 'minutes' | 'hours' })}
              >
                <option value="minutes">minutes</option>
                <option value="hours">hours</option>
              </select>
            </label>
          </>
        )}

        {(schedule.mode === 'daily' || schedule.mode === 'weekly') && (
          <label className="scheduled-field compact">
            <span>At</span>
            <input
              type="time"
              aria-label="Time of day"
              value={schedule.time}
              onChange={event => onChange({ ...schedule, time: event.target.value })}
            />
          </label>
        )}

        {schedule.mode === 'cron' && (
          <label className="scheduled-field">
            <span>Cron expression</span>
            <input
              aria-label="Cron expression"
              value={schedule.cron}
              placeholder="minute hour day month weekday"
              onChange={event => onChange({ ...schedule, cron: event.target.value })}
            />
          </label>
        )}

        <label className="scheduled-field compact">
          <span>Timezone</span>
          <select
            aria-label="Timezone"
            value={schedule.timezone}
            onChange={event => onChange({ ...schedule, timezone: event.target.value })}
          >
            {[...new Set([localZone, schedule.timezone, 'UTC'])].map(zone => (
              <option key={zone} value={zone}>{zone === localZone ? `${zone} (yours)` : zone}</option>
            ))}
          </select>
        </label>
      </div>

      {schedule.mode === 'weekly' && (
        <div className="scheduled-weekdays" role="group" aria-label="Days of week">
          {WEEKDAYS.map(day => {
            const selected = schedule.weekdays.includes(day.value)
            return (
              <button
                key={day.value}
                type="button"
                className={`scheduled-weekday ${selected ? 'active' : ''}`}
                aria-pressed={selected}
                aria-label={day.long}
                onClick={() => onChange({
                  ...schedule,
                  weekdays: selected
                    ? schedule.weekdays.filter(value => value !== day.value)
                    : [...schedule.weekdays, day.value],
                })}
              >
                {day.short}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
