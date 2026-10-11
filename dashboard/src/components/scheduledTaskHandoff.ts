import type { ScheduledSchedule } from './scheduledSchedule'

export function scheduledTaskDraft(prompt: string, sessionName: string, unixUser: string | undefined, schedule: ScheduledSchedule) {
  return {
    name: prompt.trim().split('\n')[0].slice(0, 60),
    prompt,
    targets: [{ sessionName, ...(unixUser ? { unixUser } : {}) }],
    schedule,
    enabled: true,
    paused: false,
    createdBy: 'user:dashboard',
    updatedBy: 'user:dashboard',
  }
}

export function scheduledTaskHandoff(draft: ReturnType<typeof scheduledTaskDraft>, origin: string): string {
  const endpoint = `${origin}/api/scheduled-tasks`
  return `Set up this recurring task for me in CHROTE. Refine the prompt using this session's context so each future run has clear instructions and an expected outcome. Preserve my intent, target session, cadence and timezone. Create the task after refining it; do not run the recurring work now.

Use the scheduled-tasks skill if available. Use CHROTE's scheduler, not cron or another scheduler.

Requested task (the prompt is my original input):
${JSON.stringify(draft, null, 2)}

Setup:
1. GET ${endpoint} and check for an existing matching task. Update its ID if it already exists rather than duplicating it.
2. POST ${endpoint} to create, or PATCH ${endpoint}/<id> to update. Send JSON with the schema above, replacing prompt and name with your refined wording. Include Content-Type: application/json and X-Chrote-Intent: scheduled-task. Use targets (an array), never socket paths.
3. Read back the saved task with GET ${endpoint}/<id>. Report its ID, final prompt, schedule, target and next run. It should be enabled and unpaused. Do not call run-now.

No task has been created by this handoff. If anything essential is unclear, ask me before creating it.`
}
