package schedule

import (
	"fmt"
	"time"
)

// TemplateWindow is one weekly hour in the center's timezone.
type TemplateWindow struct {
	Weekday  int
	StartMin int
	EndMin   int
}

// ParseClock reads HH:mm into minutes from midnight.
func ParseClock(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("clock")
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// FormatClock renders minutes as HH:mm.
func FormatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// TemplateOverlaps reports two windows on the same weekday that share time.
func TemplateOverlaps(windows []TemplateWindow) bool {
	for i := range windows {
		for j := i + 1; j < len(windows); j++ {
			a, b := windows[i], windows[j]
			if a.Weekday != b.Weekday {
				continue
			}
			if a.StartMin < b.EndMin && b.StartMin < a.EndMin {
				return true
			}
		}
	}
	return false
}

// ExpandHours turns the weekly template into concrete instants inside [from, to).
// A window covered by an exception or a held booking is closed.
func ExpandHours(rules []TemplateWindow, exceptions, held []Interval, from, to time.Time, loc *time.Location) (open, closed []Interval) {
	open = []Interval{}
	closed = []Interval{}
	if loc == nil {
		loc = time.UTC
	}
	startDay := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	endDay := to.In(loc)
	for day := startDay; day.Before(endDay); day = day.AddDate(0, 0, 1) {
		for _, rule := range rules {
			if rule.Weekday != int(day.Weekday()) {
				continue
			}
			window := Interval{
				Start: time.Date(day.Year(), day.Month(), day.Day(), rule.StartMin/60, rule.StartMin%60, 0, 0, loc).UTC(),
				End:   time.Date(day.Year(), day.Month(), day.Day(), rule.EndMin/60, rule.EndMin%60, 0, 0, loc).UTC(),
			}
			if !window.Start.Before(to) || !window.End.After(from) {
				continue
			}
			if covered(window, exceptions) || covered(window, held) {
				closed = append(closed, window)
				continue
			}
			open = append(open, window)
		}
	}
	return open, closed
}

func covered(window Interval, blocks []Interval) bool {
	for _, block := range blocks {
		if window.Start.Before(block.End) && block.Start.Before(window.End) {
			return true
		}
	}
	return false
}
