import { describe, expect, it } from 'vitest'
import {
  describeSchedule,
  emptyScheduleForm,
  scheduleFormError,
  scheduleFromForm,
  scheduleToForm,
} from './scheduledSchedule'

describe('scheduledSchedule', () => {
  it('compiles a daily time into a cron expression in the chosen timezone', () => {
    const form = { ...emptyScheduleForm(), mode: 'daily' as const, time: '16:00', timezone: 'Europe/Helsinki' }
    expect(scheduleFromForm(form)).toEqual({ type: 'cron', expression: '0 16 * * *', timezone: 'Europe/Helsinki' })
  })

  it('compiles weekday selections into a cron day-of-week list', () => {
    const form = { ...emptyScheduleForm(), mode: 'weekly' as const, time: '07:30', weekdays: [0, 1, 3] }
    expect(scheduleFromForm(form).expression).toBe('30 7 * * 1,3,0')
  })

  it('compiles intervals in minutes or hours', () => {
    const minutes = { ...emptyScheduleForm(), mode: 'every' as const, everyValue: '45', everyUnit: 'minutes' as const }
    const hours = { ...emptyScheduleForm(), mode: 'every' as const, everyValue: '2', everyUnit: 'hours' as const }
    expect(scheduleFromForm(minutes).everyMinutes).toBe(45)
    expect(scheduleFromForm(hours).everyMinutes).toBe(120)
  })

  it('round-trips an API schedule back into the simplest editor mode', () => {
    expect(scheduleToForm({ type: 'cron', expression: '0 16 * * *', timezone: 'Europe/Helsinki' })).toMatchObject({
      mode: 'daily',
      time: '16:00',
      timezone: 'Europe/Helsinki',
    })
    expect(scheduleToForm({ type: 'cron', expression: '30 7 * * 1,3', timezone: 'UTC' })).toMatchObject({
      mode: 'weekly',
      time: '07:30',
      weekdays: [1, 3],
    })
    expect(scheduleToForm({ type: 'interval', everyMinutes: 120, timezone: 'UTC' })).toMatchObject({
      mode: 'every',
      everyValue: '2',
      everyUnit: 'hours',
    })
    expect(scheduleToForm({ type: 'cron', expression: '*/5 9-17 * * 1-5', timezone: 'UTC' })).toMatchObject({
      mode: 'cron',
      cron: '*/5 9-17 * * 1-5',
    })
  })

  it('describes schedules in plain language', () => {
    expect(describeSchedule({ type: 'cron', expression: '0 16 * * *', timezone: 'UTC' })).toBe('Daily at 16:00')
    expect(describeSchedule({ type: 'cron', expression: '0 9 * * 1,2,3,4,5', timezone: 'UTC' })).toBe('Weekdays at 09:00')
    expect(describeSchedule({ type: 'cron', expression: '0 9 * * 6,0', timezone: 'UTC' })).toBe('Sat, Sun at 09:00')
    expect(describeSchedule({ type: 'interval', everyMinutes: 90, timezone: 'UTC' })).toBe('Every 90 minutes')
    expect(describeSchedule({ type: 'interval', everyMinutes: 60, timezone: 'UTC' })).toBe('Every hour')
    expect(describeSchedule({ type: 'cron', expression: '*/5 * * * *', timezone: 'UTC' })).toBe('Cron */5 * * * *')
  })

  it('rejects incomplete schedule input before it reaches the API', () => {
    expect(scheduleFormError({ ...emptyScheduleForm(), mode: 'every', everyValue: '0' })).toMatch(/positive/i)
    expect(scheduleFormError({ ...emptyScheduleForm(), mode: 'daily', time: '25:00' })).toMatch(/HH:MM/)
    expect(scheduleFormError({ ...emptyScheduleForm(), mode: 'weekly', weekdays: [] })).toMatch(/at least one day/i)
    expect(scheduleFormError({ ...emptyScheduleForm(), mode: 'cron', cron: '0 9 * *' })).toMatch(/five fields/i)
    expect(scheduleFormError({ ...emptyScheduleForm(), mode: 'daily', time: '16:00' })).toBeNull()
  })
})
