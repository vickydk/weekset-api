package memory

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

type heldHour struct {
	CenterID  string
	TrainerID string
	StartsAt  time.Time
	EndsAt    time.Time
}

func (s *Store) ReplaceRules(_ context.Context, centerID, branchID, trainerID string, rules []port.AvailabilityRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]port.AvailabilityRule, 0, len(s.rules)+len(rules))
	for _, rule := range s.rules {
		if rule.FitnessCenterID == centerID && rule.TrainerUserID == trainerID {
			continue
		}
		next = append(next, rule)
	}
	for _, rule := range rules {
		rule.FitnessCenterID = centerID
		rule.BranchID = branchID
		rule.TrainerUserID = trainerID
		next = append(next, rule)
	}
	s.rules = next
	return nil
}

func (s *Store) ListRules(_ context.Context, centerID, trainerID string) ([]port.AvailabilityRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.AvailabilityRule{}
	for _, rule := range s.rules {
		if rule.FitnessCenterID == centerID && rule.TrainerUserID == trainerID {
			out = append(out, rule)
		}
	}
	return out, nil
}

func (s *Store) AddException(_ context.Context, exception port.AvailabilityException) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exceptions = append(s.exceptions, exception)
	return nil
}

func (s *Store) DeleteException(_ context.Context, trainerID, exceptionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]port.AvailabilityException, 0, len(s.exceptions))
	found := false
	for _, exception := range s.exceptions {
		if exception.ID == exceptionID && exception.TrainerUserID == trainerID {
			found = true
			continue
		}
		next = append(next, exception)
	}
	if !found {
		return user.ErrNotFound
	}
	s.exceptions = next
	return nil
}

func (s *Store) ListExceptions(_ context.Context, centerID, trainerID string, from, to time.Time) ([]port.AvailabilityException, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.AvailabilityException{}
	for _, exception := range s.exceptions {
		if exception.FitnessCenterID != centerID || exception.TrainerUserID != trainerID {
			continue
		}
		if exception.StartsAt.Before(to) && exception.EndsAt.After(from) {
			out = append(out, exception)
		}
	}
	return out, nil
}

func (s *Store) ListHeld(_ context.Context, centerID, trainerID string, from, to time.Time) ([]port.HeldHour, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.HeldHour{}
	for _, hour := range s.held {
		if hour.CenterID != centerID || hour.TrainerID != trainerID {
			continue
		}
		if hour.StartsAt.Before(to) && hour.EndsAt.After(from) {
			out = append(out, port.HeldHour{StartsAt: hour.StartsAt, EndsAt: hour.EndsAt})
		}
	}
	return out, nil
}

func (s *Store) CenterZone(_ context.Context, centerID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	zone := s.zones[centerID]
	if zone == "" {
		return "UTC", nil
	}
	return zone, nil
}

func (s *Store) AssignedTrainer(_ context.Context, centerID, clientUserID string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	assignment, ok := s.assignments[memberKey(clientUserID, centerID)]
	if !ok {
		return "", "", user.ErrNotFound
	}
	return assignment.TrainerUserID, assignment.BranchID, nil
}
