package scheduleuc

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// Repeat books the same hour for eight weeks. Skip and move change one date.
func (s *Service) Repeat(ctx context.Context, userID string, start, end time.Time, headers membership.Headers) (string, error) {
	if !end.After(start) {
		return "", &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return "", err
	}
	if scope.Role != membership.RoleClient {
		return "", &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	trainerID, branchID, err := s.Schedule.AssignedTrainer(ctx, scope.FitnessCenterID, scope.UserID)
	if err != nil {
		return "", user.ErrNotFound
	}
	zone, err := s.Schedule.CenterZone(ctx, scope.FitnessCenterID)
	if err != nil {
		return "", err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	first := start.In(loc)
	duration := end.Sub(start)
	dates := schedule.ExpandWeekly(first)
	seriesID := newID()
	bookings := make([]port.Booking, 0, len(dates))
	for _, at := range dates {
		bookings = append(bookings, port.Booking{
			ID: newID(), FitnessCenterID: scope.FitnessCenterID, BranchID: branchID,
			TrainerUserID: trainerID, ClientUserID: scope.UserID, SeriesID: seriesID,
			StartsAt: at.UTC(), EndsAt: at.Add(duration).UTC(), Status: string(schedule.StatusRequested),
		})
	}
	series := port.Series{
		ID: seriesID, FitnessCenterID: scope.FitnessCenterID, BranchID: branchID,
		TrainerUserID: trainerID, ClientUserID: scope.UserID,
		Weekday: int(first.Weekday()), LocalStart: first.Format("15:04"), LocalEnd: end.In(loc).Format("15:04"),
		StartsOn: first.Format("2006-01-02"),
	}
	if err := s.hoursCover(ctx, scope.FitnessCenterID, trainerID, bookings, loc); err != nil {
		return "", err
	}
	if err := s.Schedule.SaveSeries(ctx, series, bookings); err != nil {
		return "", err
	}
	return seriesID, nil
}

func (s *Service) hoursCover(ctx context.Context, centerID, trainerID string, bookings []port.Booking, loc *time.Location) error {
	rules, err := s.Schedule.ListRules(ctx, centerID, trainerID)
	if err != nil {
		return err
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
	from := bookings[0].StartsAt
	to := bookings[len(bookings)-1].EndsAt
	exceptions, err := s.Schedule.ListExceptions(ctx, centerID, trainerID, from, to)
	if err != nil {
		return err
	}
	held, err := s.Schedule.ListHeld(ctx, centerID, trainerID, from, to)
	if err != nil {
		return err
	}
	blocks := make([]schedule.Interval, 0, len(exceptions))
	for _, exception := range exceptions {
		blocks = append(blocks, schedule.Interval{Start: exception.StartsAt, End: exception.EndsAt})
	}
	booked := make([]schedule.Interval, 0, len(held))
	for _, hour := range held {
		booked = append(booked, schedule.Interval{Start: hour.StartsAt, End: hour.EndsAt, Status: schedule.StatusConfirmed})
	}
	open, _ := schedule.ExpandHours(parsed, blocks, booked, from, to.Add(time.Second), loc)
	for _, booking := range bookings {
		matched := false
		for _, window := range open {
			if window.Start.Equal(booking.StartsAt) && window.End.Equal(booking.EndsAt) {
				matched = true
			}
		}
		if !matched {
			return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
		}
	}
	return nil
}

// Skip cancels one date. The following week stays on the pattern.
func (s *Service) Skip(ctx context.Context, userID, seriesID, occurrence string, headers membership.Headers) error {
	scope, err := s.clientSeries(ctx, userID, seriesID, headers)
	if err != nil {
		return err
	}
	day, err := time.Parse("2006-01-02", occurrence)
	if err != nil {
		return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	rows, err := s.Schedule.SeriesBookings(ctx, scope.FitnessCenterID, seriesID)
	if err != nil {
		return err
	}
	zone, _ := s.Schedule.CenterZone(ctx, scope.FitnessCenterID)
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc = time.UTC
	}
	var target *port.Booking
	for i := range rows {
		local := rows[i].StartsAt.In(loc)
		if local.Year() == day.Year() && local.Month() == day.Month() && local.Day() == day.Day() {
			target = &rows[i]
		}
	}
	if target == nil {
		return user.ErrNotFound
	}
	if err := s.Schedule.SetBookingStatus(ctx, scope.FitnessCenterID, target.ID, string(schedule.StatusCancelled)); err != nil {
		return err
	}
	return s.Schedule.SaveSeriesSkip(ctx, scope.FitnessCenterID, target.BranchID, seriesID, occurrence)
}

// Move changes one date to another hour. The other weeks stay put.
func (s *Service) Move(ctx context.Context, userID, seriesID, occurrence string, start, end time.Time, headers membership.Headers) error {
	scope, err := s.clientSeries(ctx, userID, seriesID, headers)
	if err != nil {
		return err
	}
	if !end.After(start) {
		return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	day, err := time.Parse("2006-01-02", occurrence)
	if err != nil {
		return &membership.AccessError{Code: membership.CodeValidation, Message: "check the hours"}
	}
	rows, err := s.Schedule.SeriesBookings(ctx, scope.FitnessCenterID, seriesID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.StartsAt.UTC().Format("2006-01-02") == day.Format("2006-01-02") {
			return s.Schedule.MoveBooking(ctx, scope.FitnessCenterID, row.ID, start.UTC(), end.UTC())
		}
	}
	return user.ErrNotFound
}

func (s *Service) clientSeries(ctx context.Context, userID, seriesID string, headers membership.Headers) (membership.Scope, error) {
	scope, err := s.scope(ctx, userID, headers)
	if err != nil {
		return membership.Scope{}, err
	}
	if scope.Role != membership.RoleClient {
		return membership.Scope{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	rows, err := s.Schedule.SeriesBookings(ctx, scope.FitnessCenterID, seriesID)
	if err != nil {
		return membership.Scope{}, err
	}
	if len(rows) == 0 || rows[0].ClientUserID != scope.UserID {
		return membership.Scope{}, user.ErrNotFound
	}
	return scope, nil
}
