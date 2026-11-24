package scheduleuc

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// WriteLog stores one note and its sets for a booking and marks that booking completed.
func (s *Service) WriteLog(ctx context.Context, userID, bookingID, note string, sets []port.SessionSet, headers membership.Headers) (port.SessionLog, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.SessionLog{}, err
	}
	if scope.Role != membership.RoleTrainer {
		return port.SessionLog{}, user.ErrNotFound
	}
	booking, err := s.Schedule.FindBooking(ctx, scope.FitnessCenterID, bookingID)
	if err != nil || booking.TrainerUserID != scope.UserID {
		return port.SessionLog{}, user.ErrNotFound
	}
	for _, set := range sets {
		if set.ExerciseName == "" || set.Reps < 1 || set.Unit != "kg" || set.Load == "" {
			return port.SessionLog{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the session"}
		}
	}
	existing, err := s.Schedule.FindLog(ctx, scope.FitnessCenterID, bookingID)
	id := newID()
	if err == nil {
		id = existing.ID
	}
	log := port.SessionLog{
		ID: id, BookingID: bookingID, FitnessCenterID: scope.FitnessCenterID,
		Note: note, UpdatedAt: time.Now().UTC(), Sets: sets,
	}
	if err := s.Schedule.SaveLog(ctx, log); err != nil {
		return port.SessionLog{}, err
	}
	return s.Schedule.FindLog(ctx, scope.FitnessCenterID, bookingID)
}

// ReadLog returns the log to the trainer who wrote it or the client who trained.
func (s *Service) ReadLog(ctx context.Context, userID, bookingID string, headers membership.Headers) (port.SessionLog, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.SessionLog{}, err
	}
	booking, err := s.Schedule.FindBooking(ctx, scope.FitnessCenterID, bookingID)
	if err != nil {
		return port.SessionLog{}, user.ErrNotFound
	}
	switch scope.Role {
	case membership.RoleTrainer:
		if booking.TrainerUserID != scope.UserID {
			return port.SessionLog{}, user.ErrNotFound
		}
	case membership.RoleClient:
		if booking.ClientUserID != scope.UserID {
			return port.SessionLog{}, user.ErrNotFound
		}
	default:
		return port.SessionLog{}, user.ErrNotFound
	}
	return s.Schedule.FindLog(ctx, scope.FitnessCenterID, bookingID)
}
