// Package soloplan is a day's exercises for a client training alone.
// A solo plan is not a booking and does not hold an hour.
package soloplan

// Plan is one date at one center.
type Plan struct {
	ID              string
	FitnessCenterID string
	BranchID        string
	ClientUserID    string
	TrainerUserID   string
	PlanDate        string
}

// Item is one library movement on that date.
type Item struct {
	ExerciseID  string
	Sets        int
	Reps        int
	RestSeconds int
}
