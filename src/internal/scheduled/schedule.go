package scheduled

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	scheduleTypeInterval = "interval"
	scheduleTypeCron     = "cron"
)

// NormalizeSchedule trims/defaults schedule fields and verifies that the trigger
// can compute a future run.
func NormalizeSchedule(schedule Schedule) (Schedule, error) {
	schedule.Type = strings.ToLower(strings.TrimSpace(schedule.Type))
	schedule.Expression = strings.TrimSpace(schedule.Expression)
	schedule.Timezone = strings.TrimSpace(schedule.Timezone)
	schedule.Duration = strings.TrimSpace(schedule.Duration)
	if schedule.Timezone == "" {
		schedule.Timezone = "Local"
	}

	switch schedule.Type {
	case scheduleTypeInterval:
		if _, err := intervalDuration(schedule); err != nil {
			return Schedule{}, err
		}
	case scheduleTypeCron:
		if schedule.Expression == "" {
			return Schedule{}, fmt.Errorf("%w: cron expression is required", ErrInvalid)
		}
		if _, err := scheduleLocation(schedule.Timezone); err != nil {
			return Schedule{}, err
		}
		if _, err := parseCronExpression(schedule.Expression); err != nil {
			return Schedule{}, err
		}
	default:
		return Schedule{}, fmt.Errorf("%w: schedule type must be interval or cron", ErrInvalid)
	}
	return schedule, nil
}

// NextRun returns the next firing time strictly after the supplied instant.
func NextRun(schedule Schedule, after time.Time) (time.Time, error) {
	normalized, err := NormalizeSchedule(schedule)
	if err != nil {
		return time.Time{}, err
	}

	switch normalized.Type {
	case scheduleTypeInterval:
		duration, err := intervalDuration(normalized)
		if err != nil {
			return time.Time{}, err
		}
		return after.Add(duration), nil
	case scheduleTypeCron:
		location, err := scheduleLocation(normalized.Timezone)
		if err != nil {
			return time.Time{}, err
		}
		spec, err := parseCronExpression(normalized.Expression)
		if err != nil {
			return time.Time{}, err
		}
		candidate := after.In(location).Truncate(time.Minute).Add(time.Minute)
		for i := 0; i < 5*366*24*60; i++ {
			if spec.matches(candidate) {
				return candidate, nil
			}
			candidate = candidate.Add(time.Minute)
		}
		return time.Time{}, fmt.Errorf("%w: cron expression has no match in the next five years", ErrInvalid)
	default:
		return time.Time{}, fmt.Errorf("%w: schedule type must be interval or cron", ErrInvalid)
	}
}

func intervalDuration(schedule Schedule) (time.Duration, error) {
	if schedule.EveryMinutes < 0 {
		return 0, fmt.Errorf("%w: interval everyMinutes must be positive", ErrInvalid)
	}
	if schedule.EveryMinutes > 0 {
		return time.Duration(schedule.EveryMinutes) * time.Minute, nil
	}
	if schedule.Duration == "" {
		return 0, fmt.Errorf("%w: interval everyMinutes or duration is required", ErrInvalid)
	}
	duration, err := time.ParseDuration(schedule.Duration)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%w: interval duration must be a positive duration", ErrInvalid)
	}
	return duration, nil
}

func scheduleLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "local") {
		return time.Local, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid timezone %q", ErrInvalid, name)
	}
	return location, nil
}

type cronSpec struct {
	minute cronField
	hour   cronField
	day    cronField
	month  cronField
	week   cronField
}

type cronField struct {
	any    bool
	values map[int]bool
}

func parseCronExpression(expression string) (cronSpec, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return cronSpec{}, fmt.Errorf("%w: cron expression must have five fields", ErrInvalid)
	}
	minute, err := parseCronField(fields[0], 0, 59, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("%w: invalid cron minute: %v", ErrInvalid, err)
	}
	hour, err := parseCronField(fields[1], 0, 23, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("%w: invalid cron hour: %v", ErrInvalid, err)
	}
	day, err := parseCronField(fields[2], 1, 31, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("%w: invalid cron day-of-month: %v", ErrInvalid, err)
	}
	month, err := parseCronField(fields[3], 1, 12, false)
	if err != nil {
		return cronSpec{}, fmt.Errorf("%w: invalid cron month: %v", ErrInvalid, err)
	}
	week, err := parseCronField(fields[4], 0, 7, true)
	if err != nil {
		return cronSpec{}, fmt.Errorf("%w: invalid cron day-of-week: %v", ErrInvalid, err)
	}
	return cronSpec{minute: minute, hour: hour, day: day, month: month, week: week}, nil
}

func parseCronField(raw string, minValue, maxValue int, normalizeSunday bool) (cronField, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cronField{}, fmt.Errorf("empty field")
	}
	if raw == "*" {
		return cronField{any: true}, nil
	}

	field := cronField{values: map[int]bool{}}
	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return cronField{}, fmt.Errorf("empty list item")
		}
		if err := addCronTokenValues(field.values, token, minValue, maxValue, normalizeSunday); err != nil {
			return cronField{}, err
		}
	}
	if len(field.values) == 0 {
		return cronField{}, fmt.Errorf("field has no values")
	}
	return field, nil
}

func addCronTokenValues(values map[int]bool, token string, minValue, maxValue int, normalizeSunday bool) error {
	step := 1
	base := token
	if strings.Contains(token, "/") {
		parts := strings.Split(token, "/")
		if len(parts) != 2 || parts[1] == "" {
			return fmt.Errorf("invalid step %q", token)
		}
		parsedStep, err := strconv.Atoi(parts[1])
		if err != nil || parsedStep <= 0 {
			return fmt.Errorf("invalid step %q", token)
		}
		step = parsedStep
		base = parts[0]
	}

	start, end := minValue, maxValue
	switch {
	case base == "*":
	case strings.Contains(base, "-"):
		parts := strings.Split(base, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid range %q", token)
		}
		var err error
		start, err = strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid range start %q", token)
		}
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid range end %q", token)
		}
	case base != "":
		value, err := strconv.Atoi(base)
		if err != nil {
			return fmt.Errorf("invalid value %q", token)
		}
		start, end = value, value
	default:
		return fmt.Errorf("invalid token %q", token)
	}

	if start < minValue || start > maxValue || end < minValue || end > maxValue || start > end {
		return fmt.Errorf("value out of range %q", token)
	}
	for value := start; value <= end; value += step {
		normalized := value
		if normalizeSunday && normalized == 7 {
			normalized = 0
		}
		values[normalized] = true
	}
	return nil
}

func (spec cronSpec) matches(t time.Time) bool {
	return spec.minute.matches(t.Minute()) &&
		spec.hour.matches(t.Hour()) &&
		spec.day.matches(t.Day()) &&
		spec.month.matches(int(t.Month())) &&
		spec.week.matches(int(t.Weekday()))
}

func (field cronField) matches(value int) bool {
	if field.any {
		return true
	}
	return field.values[value]
}
