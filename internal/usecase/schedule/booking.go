package scheduleuc

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// Request books one open hour as requested. A held hour returns ErrHourBooked.
func (s *Service) Request(ctx context.Context, userID string, start, end time.Time, headers membership.Headers) (port.Booking, error) {
	if !end.After(start) {
		return port.Booking{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.Booking{}, err
	}
	if scope.Role != membership.RoleClient {
		return port.Booking{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	trainerID, branchID, err := s.Schedule.AssignedTrainer(ctx, scope.FitnessCenterID, scope.UserID)
	if err != nil {
		return port.Booking{}, user.ErrNotFound
	}
	held, err := s.Schedule.ListHeld(ctx, scope.FitnessCenterID, trainerID, start, end)
	if err != nil {
		return port.Booking{}, err
	}
	existing := make([]schedule.Interval, 0, len(held))
	for _, hour := range held {
		existing = append(existing, schedule.Interval{Start: hour.StartsAt, End: hour.EndsAt, Status: schedule.StatusConfirmed})
	}
	if err := schedule.CheckOverlap(existing, schedule.Interval{Start: start, End: end, Status: schedule.StatusRequested}); err != nil {
		return port.Booking{}, err
	}
	open, err := s.Hours(ctx, userID, start, end, "", headers)
	if err != nil {
		return port.Booking{}, err
	}
	matched := false
	for _, window := range open.Windows {
		if window.Start.Equal(start.UTC()) && window.End.Equal(end.UTC()) {
			matched = true
		}
	}
	if !matched {
		return port.Booking{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	booking := port.Booking{
		ID: newID(), FitnessCenterID: scope.FitnessCenterID, BranchID: branchID,
		TrainerUserID: trainerID, ClientUserID: scope.UserID,
		StartsAt: start.UTC(), EndsAt: end.UTC(), Status: string(schedule.StatusRequested),
	}
	if err := s.Schedule.InsertBooking(ctx, booking); err != nil {
		return port.Booking{}, err
	}
	_ = s.Schedule.AddNotification(ctx, port.Notification{
		ID: newID(), FitnessCenterID: scope.FitnessCenterID, UserID: trainerID,
		Kind: "requested", BookingID: booking.ID,
	})
	return booking, nil
}

// Bookings lists the caller's hours. A client sees their own. A trainer sees the hours they train.
func (s *Service) Bookings(ctx context.Context, userID string, from, to time.Time, headers membership.Headers) ([]port.Booking, error) {
	if !to.After(from) || to.Sub(from) > maxRange {
		return nil, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return nil, err
	}
	switch scope.Role {
	case membership.RoleClient:
		return s.Schedule.ListBookings(ctx, scope.FitnessCenterID, scope.UserID, "", from, to)
	case membership.RoleTrainer:
		return s.Schedule.ListBookings(ctx, scope.FitnessCenterID, "", scope.UserID, from, to)
	default:
		return nil, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
}
