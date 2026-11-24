package scheduleuc

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// Decide confirms, declines, or cancels one booking.
// Decline and cancel release the hour. Confirm keeps it.
func (s *Service) Decide(ctx context.Context, userID, bookingID, status string, headers membership.Headers) (port.Booking, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.Booking{}, err
	}
	booking, err := s.Schedule.FindBooking(ctx, scope.FitnessCenterID, bookingID)
	if err != nil {
		return port.Booking{}, err
	}
	switch status {
	case string(schedule.StatusConfirmed), string(schedule.StatusDeclined):
		if scope.Role != membership.RoleTrainer || booking.TrainerUserID != scope.UserID {
			return port.Booking{}, user.ErrNotFound
		}
		if booking.Status != string(schedule.StatusRequested) {
			return port.Booking{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
		}
	case string(schedule.StatusCancelled):
		owns := (scope.Role == membership.RoleTrainer && booking.TrainerUserID == scope.UserID) ||
			(scope.Role == membership.RoleClient && booking.ClientUserID == scope.UserID)
		if !owns {
			return port.Booking{}, user.ErrNotFound
		}
	default:
		return port.Booking{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	if err := s.Schedule.SetBookingStatus(ctx, scope.FitnessCenterID, bookingID, status); err != nil {
		return port.Booking{}, err
	}
	booking.Status = status
	return booking, nil
}

// Timetable is who trains when. It has no note, sets, reps, or load.
func (s *Service) Timetable(ctx context.Context, userID string, from, to time.Time, headers membership.Headers) ([]port.Booking, error) {
	if !to.After(from) || to.Sub(from) > maxRange {
		return nil, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return nil, err
	}
	if scope.Role != membership.RoleManager && scope.Role != membership.RoleCenterAdmin {
		return nil, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	rows, err := s.Schedule.ListBookings(ctx, scope.FitnessCenterID, "", "", from, to)
	if err != nil {
		return nil, err
	}
	if scope.Role != membership.RoleManager {
		return rows, nil
	}
	kept := []port.Booking{}
	for _, row := range rows {
		if row.BranchID == scope.BranchID {
			kept = append(kept, row)
		}
	}
	return kept, nil
}
