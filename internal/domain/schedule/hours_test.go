package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHoursOmitsAClosedHourAndKeepsTheTemplate(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	rules := []TemplateWindow{{Weekday: int(time.Monday), StartMin: 7 * 60, EndMin: 8 * 60}}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 7)
	open, closed := ExpandHours(rules, nil, nil, from, to, loc)
	require.Len(t, open, 1)
	assert.Empty(t, closed)
	assert.Equal(t, "07:00", FormatClock(7*60))

	exception := Interval{Start: open[0].Start, End: open[0].End}
	open, closed = ExpandHours(rules, []Interval{exception}, nil, from, to, loc)
	assert.Empty(t, open)
	require.Len(t, closed, 1)
	assert.True(t, TemplateOverlaps([]TemplateWindow{
		{Weekday: 1, StartMin: 7 * 60, EndMin: 9 * 60},
		{Weekday: 1, StartMin: 8 * 60, EndMin: 10 * 60},
	}))
}

func TestHeldBookingClosesTheHour(t *testing.T) {
	loc := time.UTC
	rules := []TemplateWindow{{Weekday: int(time.Monday), StartMin: 7 * 60, EndMin: 8 * 60}}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	probe, _ := ExpandHours(rules, nil, nil, from, to, loc)
	require.NotEmpty(t, probe)
	_, closed := ExpandHours(rules, nil, []Interval{{Start: probe[0].Start, End: probe[0].End, Status: StatusConfirmed}}, from, to, loc)
	require.Len(t, closed, 1)
}
