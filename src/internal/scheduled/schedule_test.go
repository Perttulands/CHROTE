package scheduled

import (
	"testing"
	"time"
)

func TestNextRunComputesIntervalAndCron(t *testing.T) {
	after := time.Date(2026, 6, 27, 14, 29, 30, 0, time.UTC)

	intervalNext, err := NextRun(Schedule{Type: "interval", EveryMinutes: 15, Timezone: "UTC"}, after)
	if err != nil {
		t.Fatalf("interval NextRun returned error: %v", err)
	}
	wantInterval := time.Date(2026, 6, 27, 14, 44, 30, 0, time.UTC)
	if !intervalNext.Equal(wantInterval) {
		t.Fatalf("interval nextRun = %s, want %s", intervalNext.Format(time.RFC3339), wantInterval.Format(time.RFC3339))
	}

	cronNext, err := NextRun(Schedule{Type: "cron", Expression: "30 14 * * *", Timezone: "UTC"}, after)
	if err != nil {
		t.Fatalf("cron NextRun returned error: %v", err)
	}
	wantCron := time.Date(2026, 6, 27, 14, 30, 0, 0, time.UTC)
	if !cronNext.Equal(wantCron) {
		t.Fatalf("cron nextRun = %s, want %s", cronNext.Format(time.RFC3339), wantCron.Format(time.RFC3339))
	}

	afterExactCron := time.Date(2026, 6, 27, 14, 30, 0, 0, time.UTC)
	cronNext, err = NextRun(Schedule{Type: "cron", Expression: "30 14 * * *", Timezone: "UTC"}, afterExactCron)
	if err != nil {
		t.Fatalf("cron exact NextRun returned error: %v", err)
	}
	wantTomorrow := time.Date(2026, 6, 28, 14, 30, 0, 0, time.UTC)
	if !cronNext.Equal(wantTomorrow) {
		t.Fatalf("cron exact nextRun = %s, want %s", cronNext.Format(time.RFC3339), wantTomorrow.Format(time.RFC3339))
	}
}

func TestValidateScheduleRejectsInvalidIntervalAndCron(t *testing.T) {
	for _, tt := range []struct {
		name     string
		schedule Schedule
	}{
		{name: "zero interval", schedule: Schedule{Type: "interval", EveryMinutes: 0}},
		{name: "negative interval", schedule: Schedule{Type: "interval", EveryMinutes: -5}},
		{name: "invalid duration", schedule: Schedule{Type: "interval", Duration: "soon"}},
		{name: "too few cron fields", schedule: Schedule{Type: "cron", Expression: "* * * *"}},
		{name: "bad cron minute", schedule: Schedule{Type: "cron", Expression: "61 * * * *"}},
		{name: "unknown type", schedule: Schedule{Type: "solar", Expression: "sunrise"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NextRun(tt.schedule, time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)); err == nil {
				t.Fatal("NextRun returned nil error for invalid schedule")
			}
		})
	}
}
