package scheduleuc

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/exercise"
	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// AssignSolo writes a solo day. It does not create a booking.
func (s *Service) AssignSolo(ctx context.Context, userID, clientID, planDate string, items []port.SoloItem, headers membership.Headers) (port.SoloPlan, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.SoloPlan{}, err
	}
	if scope.Role != membership.RoleTrainer || planDate == "" || len(items) == 0 {
		return port.SoloPlan{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the plan"}
	}
	clients, err := s.Schedule.AssignedClients(ctx, scope.FitnessCenterID, scope.UserID)
	if err != nil {
		return port.SoloPlan{}, err
	}
	allowed := false
	for _, client := range clients {
		if client.UserID == clientID {
			allowed = true
		}
	}
	if !allowed {
		return port.SoloPlan{}, user.ErrNotFound
	}
	for _, item := range items {
		if !knownMovement(item.Slug) || item.Sets < 1 || item.Reps < 1 || item.RestSeconds < 0 {
			return port.SoloPlan{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the plan"}
		}
	}
	before, _ := s.Schedule.ListBookings(ctx, scope.FitnessCenterID, clientID, scope.UserID, time.Time{}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	plan := port.SoloPlan{
		ID: newID(), FitnessCenterID: scope.FitnessCenterID, BranchID: scope.BranchID,
		ClientUserID: clientID, TrainerUserID: scope.UserID, PlanDate: planDate, Items: items,
	}
	if err := s.Schedule.SaveSolo(ctx, plan); err != nil {
		return port.SoloPlan{}, err
	}
	after, err := s.Schedule.ListBookings(ctx, scope.FitnessCenterID, clientID, scope.UserID, time.Time{}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return port.SoloPlan{}, err
	}
	if len(after) != len(before) {
		return port.SoloPlan{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	return plan, nil
}

// Clients lists the people assigned to this trainer.
func (s *Service) Clients(ctx context.Context, userID string, headers membership.Headers) ([]port.AssignedClient, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return nil, err
	}
	if scope.Role != membership.RoleTrainer {
		return nil, user.ErrNotFound
	}
	return s.Schedule.AssignedClients(ctx, scope.FitnessCenterID, scope.UserID)
}

// Solo returns the plan for that date. A missing plan is not found, not an empty booking.
func (s *Service) Solo(ctx context.Context, userID, clientID, planDate string, headers membership.Headers) (port.SoloPlan, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.SoloPlan{}, err
	}
	target := scope.UserID
	if scope.Role == membership.RoleTrainer {
		target = clientID
	} else if scope.Role != membership.RoleClient || (clientID != "" && clientID != scope.UserID) {
		return port.SoloPlan{}, user.ErrNotFound
	}
	return s.Schedule.FindSolo(ctx, scope.FitnessCenterID, target, planDate)
}

func knownMovement(slug string) bool {
	for _, movement := range exercise.Catalog {
		if movement.Slug == slug {
			return true
		}
	}
	return false
}
