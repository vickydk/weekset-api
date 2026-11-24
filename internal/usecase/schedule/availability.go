package scheduleuc

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

const maxRange = 14 * 24 * time.Hour

// ErrRange is a from/to window longer than 14 days.
var ErrRange = fmt.Errorf("range")

// Service publishes a trainer's hours.
type Service struct {
	Schedule port.ScheduleStore
	Users    port.UserRepository
}

// WindowInput is one template row.
type WindowInput struct {
	Weekday    int
	LocalStart string
	LocalEnd   string
}

// Hours is the open list, and the closed list for the trainer who owns it.
type Hours struct {
	Windows  []schedule.Interval
	Closed   []ClosedWindow
	Template []port.AvailabilityRule
	Trainer  bool
}

// ClosedWindow is one exception the trainer can reopen.
type ClosedWindow struct {
	ID    string
	Start time.Time
	End   time.Time
}

// ReplaceTemplate replaces the trainer's weekly hours. An empty list clears them.
func (s *Service) ReplaceTemplate(ctx context.Context, userID string, windows []WindowInput, headers membership.Headers) error {
	scope, err := s.trainer(ctx, userID, headers)
	if err != nil {
		return err
	}
	parsed := make([]schedule.TemplateWindow, 0, len(windows))
	rules := make([]port.AvailabilityRule, 0, len(windows))
	for _, window := range windows {
		if window.Weekday < 0 || window.Weekday > 6 {
			return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
		}
		start, err := schedule.ParseClock(window.LocalStart)
		if err != nil {
			return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
		}
		end, err := schedule.ParseClock(window.LocalEnd)
		if err != nil || end <= start {
			return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
		}
		parsed = append(parsed, schedule.TemplateWindow{Weekday: window.Weekday, StartMin: start, EndMin: end})
		rules = append(rules, port.AvailabilityRule{
			ID: newID(), Weekday: window.Weekday, LocalStart: window.LocalStart, LocalEnd: window.LocalEnd,
		})
	}
	if schedule.TemplateOverlaps(parsed) {
		return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	return s.Schedule.ReplaceRules(ctx, scope.FitnessCenterID, scope.BranchID, scope.UserID, rules)
}

// CloseHour records one exception. The template stays.
func (s *Service) CloseHour(ctx context.Context, userID string, start, end time.Time, headers membership.Headers) (string, error) {
	scope, err := s.trainer(ctx, userID, headers)
	if err != nil {
		return "", err
	}
	if !end.After(start) {
		return "", &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	id := newID()
	err = s.Schedule.AddException(ctx, port.AvailabilityException{
		ID: id, FitnessCenterID: scope.FitnessCenterID, BranchID: scope.BranchID, TrainerUserID: scope.UserID,
		StartsAt: start.UTC(), EndsAt: end.UTC(),
	})
	return id, err
}

// ReopenHour removes one closure unless a booking still occupies it.
func (s *Service) ReopenHour(ctx context.Context, userID, exceptionID string, headers membership.Headers) error {
	scope, err := s.trainer(ctx, userID, headers)
	if err != nil {
		return err
	}
	exceptions, err := s.Schedule.ListExceptions(ctx, scope.FitnessCenterID, scope.UserID, time.Time{}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	var target *port.AvailabilityException
	for i := range exceptions {
		if exceptions[i].ID == exceptionID {
			target = &exceptions[i]
			break
		}
	}
	if target == nil {
		return user.ErrNotFound
	}
	held, err := s.Schedule.ListHeld(ctx, scope.FitnessCenterID, scope.UserID, target.StartsAt, target.EndsAt)
	if err != nil {
		return err
	}
	if len(held) > 0 {
		return schedule.ErrHourBooked
	}
	return s.Schedule.DeleteException(ctx, userID, exceptionID)
}

// Hours returns open windows. The trainer also receives closed windows and the template.
func (s *Service) Hours(ctx context.Context, userID string, from, to time.Time, trainerID string, headers membership.Headers) (Hours, error) {
	if !to.After(from) || to.Sub(from) > maxRange {
		return Hours{}, &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return Hours{}, err
	}
	target := scope.UserID
	own := scope.Role == membership.RoleTrainer
	if scope.Role == membership.RoleClient {
		assigned, _, err := s.Schedule.AssignedTrainer(ctx, scope.FitnessCenterID, scope.UserID)
		if err != nil {
			return Hours{}, user.ErrNotFound
		}
		if trainerID != "" && trainerID != assigned {
			return Hours{}, user.ErrNotFound
		}
		target = assigned
		own = false
	} else if scope.Role != membership.RoleTrainer {
		return Hours{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	} else if trainerID != "" && trainerID != scope.UserID {
		return Hours{}, user.ErrNotFound
	}
	return s.expand(ctx, scope.FitnessCenterID, target, from, to, own)
}

func (s *Service) expand(ctx context.Context, centerID, trainerID string, from, to time.Time, own bool) (Hours, error) {
	zone, err := s.Schedule.CenterZone(ctx, centerID)
	if err != nil {
		return Hours{}, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	rules, err := s.Schedule.ListRules(ctx, centerID, trainerID)
	if err != nil {
		return Hours{}, err
	}
	exceptions, err := s.Schedule.ListExceptions(ctx, centerID, trainerID, from, to)
	if err != nil {
		return Hours{}, err
	}
	held, err := s.Schedule.ListHeld(ctx, centerID, trainerID, from, to)
	if err != nil {
		return Hours{}, err
	}
	parsed := make([]schedule.TemplateWindow, 0, len(rules))
	for _, rule := range rules {
		start, err := schedule.ParseClock(rule.LocalStart)
		if err != nil {
			continue
		}
		end, err := schedule.ParseClock(rule.LocalEnd)
		if err != nil {
			continue
		}
		parsed = append(parsed, schedule.TemplateWindow{Weekday: rule.Weekday, StartMin: start, EndMin: end})
	}
	blocks := make([]schedule.Interval, 0, len(exceptions))
	for _, exception := range exceptions {
		blocks = append(blocks, schedule.Interval{Start: exception.StartsAt, End: exception.EndsAt})
	}
	booked := make([]schedule.Interval, 0, len(held))
	for _, hour := range held {
		booked = append(booked, schedule.Interval{Start: hour.StartsAt, End: hour.EndsAt, Status: schedule.StatusConfirmed})
	}
	open, _ := schedule.ExpandHours(parsed, blocks, booked, from, to, loc)
	hours := Hours{Windows: open, Trainer: own}
	if own {
		closed := make([]ClosedWindow, 0, len(exceptions))
		for _, exception := range exceptions {
			closed = append(closed, ClosedWindow{ID: exception.ID, Start: exception.StartsAt, End: exception.EndsAt})
		}
		hours.Closed = closed
		hours.Template = rules
	}
	return hours, nil
}

func (s *Service) trainer(ctx context.Context, userID string, headers membership.Headers) (membership.Scope, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return membership.Scope{}, err
	}
	if scope.Role != membership.RoleTrainer {
		return membership.Scope{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	return scope, nil
}

func (s *Service) scope(ctx context.Context, userID string, headers membership.Headers) (membership.Scope, error) {
	rows, err := s.Users.Memberships(ctx, userID)
	if err != nil {
		return membership.Scope{}, err
	}
	if len(rows) == 0 {
		return membership.Scope{}, user.ErrUnauthorized
	}
	actor := membership.Actor{UserID: userID}
	for _, row := range rows {
		item := membership.Membership{
			FitnessCenterID: row.FitnessCenterID,
			Role:            membership.Role(row.Role),
			Status:          membership.Status(row.Status),
			BranchID:        row.BranchID,
		}
		for _, branch := range row.Branches {
			item.BranchIDs = append(item.BranchIDs, branch.ID)
		}
		actor.Memberships = append(actor.Memberships, item)
	}
	return membership.Resolve(actor, headers, membership.BodyClaim{})
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
