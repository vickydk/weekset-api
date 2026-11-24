package memory

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

type heldHour struct {
	CenterID  string
	TrainerID string
	StartsAt  time.Time
	EndsAt    time.Time
}

type auditEvent struct {
	ID       string
	CenterID string
	ActorID  string
	Action   string
	Entity   string
	EntityID string
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

func (s *Store) InsertBooking(_ context.Context, booking port.Booking) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if schedule.HoldsSlot(schedule.Status(booking.Status)) {
		for _, hour := range s.held {
			if hour.CenterID != booking.FitnessCenterID || hour.TrainerID != booking.TrainerUserID {
				continue
			}
			if booking.StartsAt.Before(hour.EndsAt) && hour.StartsAt.Before(booking.EndsAt) {
				return schedule.ErrHourBooked
			}
		}
		s.held = append(s.held, heldHour{
			CenterID: booking.FitnessCenterID, TrainerID: booking.TrainerUserID,
			StartsAt: booking.StartsAt, EndsAt: booking.EndsAt,
		})
	}
	s.bookings = append(s.bookings, booking)
	return nil
}

func (s *Store) ListBookings(_ context.Context, centerID, clientID, trainerID string, from, to time.Time) ([]port.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.Booking{}
	for _, booking := range s.bookings {
		if booking.FitnessCenterID != centerID {
			continue
		}
		if clientID != "" && booking.ClientUserID != clientID {
			continue
		}
		if trainerID != "" && booking.TrainerUserID != trainerID {
			continue
		}
		if !booking.StartsAt.Before(to) || !booking.EndsAt.After(from) {
			continue
		}
		if account, ok := s.users[booking.ClientUserID]; ok {
			booking.ClientName = account.DisplayName
		}
		if account, ok := s.users[booking.TrainerUserID]; ok {
			booking.TrainerName = account.DisplayName
		}
		out = append(out, booking)
	}
	return out, nil
}

func (s *Store) FindBooking(_ context.Context, centerID, bookingID string) (port.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, booking := range s.bookings {
		if booking.FitnessCenterID == centerID && booking.ID == bookingID {
			if account, ok := s.users[booking.ClientUserID]; ok {
				booking.ClientName = account.DisplayName
			}
			if account, ok := s.users[booking.TrainerUserID]; ok {
				booking.TrainerName = account.DisplayName
			}
			return booking, nil
		}
	}
	return port.Booking{}, user.ErrNotFound
}

func (s *Store) SetBookingStatus(_ context.Context, centerID, bookingID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, booking := range s.bookings {
		if booking.FitnessCenterID != centerID || booking.ID != bookingID {
			continue
		}
		if schedule.HoldsSlot(schedule.Status(booking.Status)) && !schedule.HoldsSlot(schedule.Status(status)) {
			kept := make([]heldHour, 0, len(s.held))
			removed := false
			for _, hour := range s.held {
				if !removed && hour.TrainerID == booking.TrainerUserID && hour.StartsAt.Equal(booking.StartsAt) && hour.EndsAt.Equal(booking.EndsAt) {
					removed = true
					continue
				}
				kept = append(kept, hour)
			}
			s.held = kept
		}
		booking.Status = status
		s.bookings[i] = booking
		return nil
	}
	return user.ErrNotFound
}

func (s *Store) SaveSeries(_ context.Context, series port.Series, bookings []port.Booking) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, booking := range bookings {
		if !schedule.HoldsSlot(schedule.Status(booking.Status)) {
			continue
		}
		for _, hour := range s.held {
			if hour.TrainerID == booking.TrainerUserID && booking.StartsAt.Before(hour.EndsAt) && hour.StartsAt.Before(booking.EndsAt) {
				return schedule.ErrHourBooked
			}
		}
	}
	s.series = append(s.series, series)
	for _, booking := range bookings {
		if schedule.HoldsSlot(schedule.Status(booking.Status)) {
			s.held = append(s.held, heldHour{
				CenterID: booking.FitnessCenterID, TrainerID: booking.TrainerUserID,
				StartsAt: booking.StartsAt, EndsAt: booking.EndsAt,
			})
		}
		s.bookings = append(s.bookings, booking)
	}
	return nil
}

func (s *Store) SeriesBookings(_ context.Context, centerID, seriesID string) ([]port.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.Booking{}
	for _, booking := range s.bookings {
		if booking.FitnessCenterID == centerID && booking.SeriesID == seriesID {
			out = append(out, booking)
		}
	}
	return out, nil
}

func (s *Store) MoveBooking(_ context.Context, centerID, bookingID string, start, end time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, booking := range s.bookings {
		if booking.FitnessCenterID != centerID || booking.ID != bookingID {
			continue
		}
		for _, hour := range s.held {
			if hour.TrainerID != booking.TrainerUserID || (hour.StartsAt.Equal(booking.StartsAt) && hour.EndsAt.Equal(booking.EndsAt)) {
				continue
			}
			if start.Before(hour.EndsAt) && hour.StartsAt.Before(end) {
				return schedule.ErrHourBooked
			}
		}
		kept := make([]heldHour, 0, len(s.held))
		for _, hour := range s.held {
			if hour.TrainerID == booking.TrainerUserID && hour.StartsAt.Equal(booking.StartsAt) && hour.EndsAt.Equal(booking.EndsAt) {
				continue
			}
			kept = append(kept, hour)
		}
		kept = append(kept, heldHour{CenterID: centerID, TrainerID: booking.TrainerUserID, StartsAt: start, EndsAt: end})
		s.held = kept
		booking.StartsAt = start
		booking.EndsAt = end
		s.bookings[i] = booking
		return nil
	}
	return user.ErrNotFound
}

func (s *Store) SaveSeriesSkip(_ context.Context, centerID, branchID, seriesID, occurrence string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skips = append(s.skips, occurrence+"|"+seriesID)
	return nil
}

func (s *Store) AddNotification(_ context.Context, notice port.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if notice.CreatedAt.IsZero() {
		notice.CreatedAt = time.Now().UTC()
	}
	s.notices = append(s.notices, notice)
	return nil
}

func (s *Store) ListNotifications(_ context.Context, centerID, userID string) ([]port.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.Notification{}
	for _, notice := range s.notices {
		if notice.FitnessCenterID == centerID && notice.UserID == userID {
			out = append(out, notice)
		}
	}
	return out, nil
}

func (s *Store) ReadNotification(_ context.Context, centerID, userID, noticeID string) (port.Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, notice := range s.notices {
		if notice.ID == noticeID && notice.FitnessCenterID == centerID && notice.UserID == userID {
			notice.Read = true
			s.notices[i] = notice
			return notice, nil
		}
	}
	return port.Notification{}, user.ErrNotFound
}

func (s *Store) SaveLog(_ context.Context, log port.SessionLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var booking *port.Booking
	for i := range s.bookings {
		if s.bookings[i].ID == log.BookingID && s.bookings[i].FitnessCenterID == log.FitnessCenterID {
			booking = &s.bookings[i]
			break
		}
	}
	if booking == nil {
		return user.ErrNotFound
	}
	log.BranchID = booking.BranchID
	if log.UpdatedAt.IsZero() {
		log.UpdatedAt = time.Now().UTC()
	}
	replaced := false
	for i := range s.logs {
		if s.logs[i].BookingID == log.BookingID {
			log.ID = s.logs[i].ID
			s.logs[i] = log
			replaced = true
		}
	}
	if !replaced {
		if log.ID == "" {
			return user.ErrNotFound
		}
		s.logs = append(s.logs, log)
	}
	if schedule.HoldsSlot(schedule.Status(booking.Status)) {
		kept := make([]heldHour, 0, len(s.held))
		for _, hour := range s.held {
			if hour.TrainerID == booking.TrainerUserID && hour.StartsAt.Equal(booking.StartsAt) {
				continue
			}
			kept = append(kept, hour)
		}
		s.held = kept
	}
	booking.Status = string(schedule.StatusCompleted)
	s.audits = append(s.audits, auditEvent{
		ID: log.ID, CenterID: log.FitnessCenterID, ActorID: booking.TrainerUserID,
		Action: "session_log", Entity: "session_log", EntityID: log.ID,
	})
	return nil
}

func (s *Store) FindLog(_ context.Context, centerID, bookingID string) (port.SessionLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, log := range s.logs {
		if log.FitnessCenterID == centerID && log.BookingID == bookingID {
			return log, nil
		}
	}
	return port.SessionLog{}, user.ErrNotFound
}

func (s *Store) SaveSolo(_ context.Context, plan port.SoloPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]port.SoloPlan, 0, len(s.solos))
	for _, existing := range s.solos {
		if existing.FitnessCenterID == plan.FitnessCenterID && existing.ClientUserID == plan.ClientUserID && existing.PlanDate == plan.PlanDate {
			continue
		}
		next = append(next, existing)
	}
	next = append(next, plan)
	s.solos = next
	return nil
}

func (s *Store) FindSolo(_ context.Context, centerID, clientID, planDate string) (port.SoloPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, plan := range s.solos {
		if plan.FitnessCenterID == centerID && plan.ClientUserID == clientID && plan.PlanDate == planDate {
			return plan, nil
		}
	}
	return port.SoloPlan{}, user.ErrNotFound
}

func (s *Store) AssignedClients(_ context.Context, centerID, trainerID string) ([]port.AssignedClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.AssignedClient{}
	for _, assignment := range s.assignments {
		if assignment.FitnessCenterID != centerID || assignment.TrainerUserID != trainerID {
			continue
		}
		name := assignment.ClientUserID
		if account, ok := s.users[assignment.ClientUserID]; ok {
			name = account.DisplayName
		}
		out = append(out, port.AssignedClient{UserID: assignment.ClientUserID, DisplayName: name})
	}
	return out, nil
}

func (s *Store) ListAudits(_ context.Context, centerID string) ([]port.AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.AuditEvent{}
	for _, event := range s.audits {
		if event.CenterID == centerID {
			out = append(out, port.AuditEvent{
				ActorID: event.ActorID, Action: event.Action, EntityType: event.Entity, EntityID: event.EntityID,
			})
		}
	}
	return out, nil
}
