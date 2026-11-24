// Package sessionlog is the workout written on one confirmed booking.
// Removing a membership does not remove the log.
package sessionlog

// Log is the note and the sets for one booking.
type Log struct {
	ID              string
	BookingID       string
	FitnessCenterID string
	BranchID        string
	Note            string
}

// Set is one recorded effort. The name is free text, not a library slug.
type Set struct {
	ExerciseName string
	Reps         int
	Load         string
	Unit         string
}
