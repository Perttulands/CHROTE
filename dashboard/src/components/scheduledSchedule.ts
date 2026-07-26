// Schedule model for the Scheduled tab. The API speaks interval/cron; the UI
// speaks Every/Daily/Weekly/Cron. Everything in this module is pure so the
// translation is testable without rendering the view.

export interface ScheduledSchedule {
  type: 'interval' | 'cron'
  expression?: string
  timezone: string
  everyMinutes?: number
}

export type ScheduleMode = 'every' | 'daily' | 'weekly' | 'cron'

export interface ScheduleForm {
  mode: ScheduleMode
  everyValue: string
  everyUnit: 'minutes' | 'hours'
  time: string
  weekdays: number[]
  cron: string
  timezone: string
}

export const WEEKDAYS: { value: number; short: string; long: string }[] = [
  { value: 1, short: 'Mon', long: 'Monday' },
  { value: 2, short: 'Tue', long: 'Tuesday' },
  { value: 3, short: 'Wed', long: 'Wednesday' },
  { value: 4, short: 'Thu', long: 'Thursday' },
  { value: 5, short: 'Fri', long: 'Friday' },
  { value: 6, short: 'Sat', long: 'Saturday' },
  { value: 0, short: 'Sun', long: 'Sunday' },
]

const DAILY_CRON = /^(\d{1,2}) (\d{1,2}) \* \* \*$/
const WEEKLY_CRON = /^(\d{1,2}) (\d{1,2}) \* \* ([0-7](?:,[0-7])*)$/

// browserTimezone resolves the operator's IANA zone so nobody has to type one.
export function browserTimezone(): string {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
    if (zone) return zone
  } catch {
    // Intl is unavailable in some embedded webviews; fall through.
  }
  return 'UTC'
}

export function emptyScheduleForm(): ScheduleForm {
  return {
    mode: 'daily',
    everyValue: '30',
    everyUnit: 'minutes',
    time: '09:00',
    weekdays: [1, 2, 3, 4, 5],
    cron: '0 9 * * *',
    timezone: browserTimezone(),
  }
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

function parseTime(time: string): { hour: number; minute: number } | null {
  const match = /^(\d{1,2}):(\d{2})$/.exec(time.trim())
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  if (!Number.isInteger(hour) || !Number.isInteger(minute)) return null
  if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return null
  return { hour, minute }
}

// sortWeekdays keeps Monday-first display order with Sunday last, matching WEEKDAYS.
function sortWeekdays(days: number[]): number[] {
  const order = WEEKDAYS.map(day => day.value)
  return [...new Set(days)].sort((a, b) => order.indexOf(a) - order.indexOf(b))
}

export function scheduleFormError(form: ScheduleForm): string | null {
  if (!form.timezone.trim()) return 'Timezone is required.'
  switch (form.mode) {
    case 'every': {
      const value = Number(form.everyValue)
      if (!Number.isFinite(value) || value <= 0) return 'Interval must be a positive number.'
      if (form.everyUnit === 'hours' && value > 24 * 30) return 'Interval is too long.'
      return null
    }
    case 'daily':
      return parseTime(form.time) ? null : 'Time must be HH:MM.'
    case 'weekly':
      if (!parseTime(form.time)) return 'Time must be HH:MM.'
      return form.weekdays.length > 0 ? null : 'Pick at least one day.'
    case 'cron':
      return form.cron.trim().split(/\s+/).length === 5 ? null : 'Cron needs five fields: minute hour day month weekday.'
    default:
      return 'Unknown schedule type.'
  }
}

export function scheduleFromForm(form: ScheduleForm): ScheduledSchedule {
  const timezone = form.timezone.trim() || 'UTC'
  if (form.mode === 'every') {
    const value = Number(form.everyValue)
    const everyMinutes = form.everyUnit === 'hours' ? Math.round(value * 60) : Math.round(value)
    return { type: 'interval', everyMinutes, timezone }
  }
  if (form.mode === 'cron') {
    return { type: 'cron', expression: form.cron.trim(), timezone }
  }
  const time = parseTime(form.time) ?? { hour: 9, minute: 0 }
  const weekdays = form.mode === 'weekly' ? sortWeekdays(form.weekdays).join(',') : '*'
  return { type: 'cron', expression: `${time.minute} ${time.hour} * * ${weekdays}`, timezone }
}

export function scheduleToForm(schedule: ScheduledSchedule | undefined): ScheduleForm {
  const base = emptyScheduleForm()
  if (!schedule) return base
  const timezone = schedule.timezone?.trim() || base.timezone

  if (schedule.type === 'interval') {
    const minutes = schedule.everyMinutes && schedule.everyMinutes > 0 ? schedule.everyMinutes : 30
    const useHours = minutes >= 60 && minutes % 60 === 0
    return {
      ...base,
      mode: 'every',
      everyValue: String(useHours ? minutes / 60 : minutes),
      everyUnit: useHours ? 'hours' : 'minutes',
      timezone,
    }
  }

  const expression = schedule.expression?.trim() || ''
  const daily = DAILY_CRON.exec(expression)
  if (daily) {
    return { ...base, mode: 'daily', time: `${pad(Number(daily[2]))}:${pad(Number(daily[1]))}`, cron: expression, timezone }
  }
  const weekly = WEEKLY_CRON.exec(expression)
  if (weekly) {
    const weekdays = sortWeekdays(weekly[3].split(',').map(day => (Number(day) === 7 ? 0 : Number(day))))
    return {
      ...base,
      mode: 'weekly',
      time: `${pad(Number(weekly[2]))}:${pad(Number(weekly[1]))}`,
      weekdays,
      cron: expression,
      timezone,
    }
  }
  return { ...base, mode: 'cron', cron: expression || base.cron, timezone }
}

// describeSchedule renders the plain-language summary shown on task cards.
export function describeSchedule(schedule: ScheduledSchedule | undefined): string {
  if (!schedule) return 'No schedule'
  if (schedule.type === 'interval') {
    const minutes = schedule.everyMinutes ?? 0
    if (minutes <= 0) return 'No schedule'
    if (minutes % 60 === 0 && minutes >= 60) {
      const hours = minutes / 60
      return hours === 1 ? 'Every hour' : `Every ${hours} hours`
    }
    return minutes === 1 ? 'Every minute' : `Every ${minutes} minutes`
  }

  const expression = schedule.expression?.trim() || ''
  const daily = DAILY_CRON.exec(expression)
  if (daily) return `Daily at ${pad(Number(daily[2]))}:${pad(Number(daily[1]))}`
  const weekly = WEEKLY_CRON.exec(expression)
  if (weekly) {
    const days = sortWeekdays(weekly[3].split(',').map(day => (Number(day) === 7 ? 0 : Number(day))))
    const labels = days.map(day => WEEKDAYS.find(weekday => weekday.value === day)?.short ?? String(day))
    const everyWeekday = days.length === 5 && [1, 2, 3, 4, 5].every(day => days.includes(day))
    const when = everyWeekday ? 'Weekdays' : labels.join(', ')
    return `${when} at ${pad(Number(weekly[2]))}:${pad(Number(weekly[1]))}`
  }
  return expression ? `Cron ${expression}` : 'No schedule'
}
