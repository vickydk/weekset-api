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
	InsertBooking(ctx context.Context, booking Booking) error
	ListBookings(ctx context.Context, centerID, clientID, trainerID string, from, to time.Time) ([]Booking, error)
	FindBooking(ctx context.Context, centerID, bookingID string) (Booking, error)
	SetBookingStatus(ctx context.Context, centerID, bookingID, status string) error
	SaveSeries(ctx context.Context, series Series, bookings []Booking) error
	SeriesBookings(ctx context.Context, centerID, seriesID string) ([]Booking, error)
	MoveBooking(ctx context.Context, centerID, bookingID string, start, end time.Time) error
	SaveSeriesSkip(ctx context.Context, centerID, branchID, seriesID, occurrence string) error
	AddNotification(ctx context.Context, notice Notification) error
	ListNotifications(ctx context.Context, centerID, userID string) ([]Notification, error)
	ReadNotification(ctx context.Context, centerID, userID, noticeID string) (Notification, error)
}

// Booking is one hour a client asked for.
type Booking struct {
	ID              string
	FitnessCenterID string
	BranchID        string
	TrainerUserID   string
	ClientUserID    string
	TrainerName     string
	ClientName      string
	SeriesID        string
	StartsAt        time.Time
	EndsAt          time.Time
	Status          string
}

// Series is a fixed weekly pattern.
type Series struct {
	ID              string
	FitnessCenterID string
	BranchID        string
	TrainerUserID   string
	ClientUserID    string
	Weekday         int
	LocalStart      string
	LocalEnd        string
	StartsOn        string
}

// Notification is an in-app schedule update. Kind is the event. BookingID is the session.
type Notification struct {
	ID              string
	FitnessCenterID string
	UserID          string
	Kind            string
	BookingID       string
	Read            bool
	CreatedAt       time.Time
}
