package port

import (
	"context"
	"time"
)

// AvailabilityRule is one weekly hour for a trainer.
type AvailabilityRule struct {
	ID              string
	FitnessCenterID string
	BranchID        string
	TrainerUserID   string
	Weekday         int
	LocalStart      string
	LocalEnd        string
}

// AvailabilityException closes one concrete hour.
type AvailabilityException struct {
	ID              string
	FitnessCenterID string
	BranchID        string
	TrainerUserID   string
	StartsAt        time.Time
	EndsAt          time.Time
}

// HeldHour is a requested or confirmed booking.
type HeldHour struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// ScheduleStore keeps hours and, later, the bookings that occupy them.
type ScheduleStore interface {
	ReplaceRules(ctx context.Context, centerID, branchID, trainerID string, rules []AvailabilityRule) error
	ListRules(ctx context.Context, centerID, trainerID string) ([]AvailabilityRule, error)
	AddException(ctx context.Context, exception AvailabilityException) error
	DeleteException(ctx context.Context, trainerID, exceptionID string) error
	ListExceptions(ctx context.Context, centerID, trainerID string, from, to time.Time) ([]AvailabilityException, error)
	ListHeld(ctx context.Context, centerID, trainerID string, from, to time.Time) ([]HeldHour, error)
	CenterZone(ctx context.Context, centerID string) (string, error)
	AssignedTrainer(ctx context.Context, centerID, clientUserID string) (trainerID, branchID string, err error)
}
