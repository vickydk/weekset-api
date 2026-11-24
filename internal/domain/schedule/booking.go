// Package schedule is the shared week: open hours, bookings, and a fixed pattern.
package schedule

import (
	"errors"
	"time"
)

// Status is a booking status. Requested and confirmed hold the hour.
type Status string

const (
	StatusRequested Status = "requested"
	StatusConfirmed Status = "confirmed"
	StatusDeclined  Status = "declined"
	StatusCancelled Status = "cancelled"
	StatusCompleted Status = "completed"
)

// ErrHourBooked is the conflict when a trainer already holds that hour.
var ErrHourBooked = errors.New("hour is already booked")

// SeriesWeeks is how many concrete dates a fixed week generates.
const SeriesWeeks = 8

// Interval is one booking range. The end is exclusive.
type Interval struct {
	Start  time.Time
	End    time.Time
	Status Status
}

// HoldsSlot reports whether the status blocks the trainer's hour.
func HoldsSlot(status Status) bool {
	return status == StatusRequested || status == StatusConfirmed
}

// CheckOverlap rejects a requested or confirmed hour that meets one already held.
// Declined, cancelled, and completed hours stay free.
func CheckOverlap(existing []Interval, next Interval) error {
	if !HoldsSlot(next.Status) {
		return nil
	}
	for _, booked := range existing {
		if !HoldsSlot(booked.Status) {
			continue
		}
		if next.Start.Before(booked.End) && booked.Start.Before(next.End) {
			return ErrHourBooked
		}
	}
	return nil
}

// ExpandWeekly returns one instant per week for SeriesWeeks, starting at start.
func ExpandWeekly(start time.Time) []time.Time {
	dates := make([]time.Time, SeriesWeeks)
	for week := 0; week < SeriesWeeks; week++ {
		dates[week] = start.AddDate(0, 0, 7*week)
	}
	return dates
}
