package scheduleuc

import (
	"context"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/port"
)

// Inbox is the caller's in-app schedule updates.
func (s *Service) Inbox(ctx context.Context, userID string, headers membership.Headers) ([]port.Notification, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return nil, err
	}
	return s.Schedule.ListNotifications(ctx, scope.FitnessCenterID, scope.UserID)
}

// OpenNotice marks one update read and returns the session it points at.
func (s *Service) OpenNotice(ctx context.Context, userID, noticeID string, headers membership.Headers) (port.Notification, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return port.Notification{}, err
	}
	return s.Schedule.ReadNotification(ctx, scope.FitnessCenterID, scope.UserID, noticeID)
}
