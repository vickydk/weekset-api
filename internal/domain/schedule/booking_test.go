package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hour(clock string) time.Time {
	parsed, err := time.Parse(time.RFC3339, "2026-11-24T"+clock+":00Z")
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestOverlapHoldsRequestedAndConfirmed(t *testing.T) {
	held := []Interval{{
		Start:  hour("07:00"),
		End:    hour("08:00"),
		Status: StatusConfirmed,
	}}
	err := CheckOverlap(held, Interval{
		Start:  hour("07:30"),
		End:    hour("08:30"),
		Status: StatusRequested,
	})
	assert.ErrorIs(t, err, ErrHourBooked)
	assert.EqualError(t, err, "hour is already booked")
}

func TestDeclinedHourDoesNotBlock(t *testing.T) {
	held := []Interval{{
		Start:  hour("07:00"),
		End:    hour("08:00"),
		Status: StatusDeclined,
	}}
	err := CheckOverlap(held, Interval{
		Start:  hour("07:00"),
		End:    hour("08:00"),
		Status: StatusConfirmed,
	})
	assert.NoError(t, err)
}

func TestCancelledAndCompletedDoNotBlock(t *testing.T) {
	start := hour("07:00")
	end := hour("08:00")
	for _, status := range []Status{StatusCancelled, StatusCompleted} {
		err := CheckOverlap([]Interval{{Start: start, End: end, Status: status}}, Interval{
			Start:  start,
			End:    end,
			Status: StatusRequested,
		})
		assert.NoError(t, err, status)
	}
}

func TestTouchingHoursDoNotOverlap(t *testing.T) {
	err := CheckOverlap([]Interval{{
		Start:  hour("07:00"),
		End:    hour("08:00"),
		Status: StatusConfirmed,
	}}, Interval{
		Start:  hour("08:00"),
		End:    hour("09:00"),
		Status: StatusRequested,
	})
	assert.NoError(t, err)
}

func TestSeriesExpandsToEightWeeks(t *testing.T) {
	start := hour("07:00")
	dates := ExpandWeekly(start)
	require.Len(t, dates, SeriesWeeks)
	assert.True(t, dates[0].Equal(start))
	assert.True(t, dates[7].Equal(start.AddDate(0, 0, 49)))
}
