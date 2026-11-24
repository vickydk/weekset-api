package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func (s *Store) ReplaceRules(ctx context.Context, centerID, branchID, trainerID string, rules []port.AvailabilityRule) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM availability_rules WHERE fitness_center_id = $1 AND trainer_user_id = $2`, centerID, trainerID); err != nil {
		return err
	}
	for _, rule := range rules {
		_, err := tx.Exec(ctx, `
			INSERT INTO availability_rules (id, fitness_center_id, branch_id, trainer_user_id, weekday, local_start, local_end)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			rule.ID, centerID, branchID, trainerID, rule.Weekday, rule.LocalStart, rule.LocalEnd)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListRules(ctx context.Context, centerID, trainerID string) ([]port.AvailabilityRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, fitness_center_id::text, branch_id::text, trainer_user_id::text, weekday, local_start::text, local_end::text
		FROM availability_rules WHERE fitness_center_id = $1 AND trainer_user_id = $2
		ORDER BY weekday, local_start`, centerID, trainerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.AvailabilityRule{}
	for rows.Next() {
		var rule port.AvailabilityRule
		if err := rows.Scan(&rule.ID, &rule.FitnessCenterID, &rule.BranchID, &rule.TrainerUserID, &rule.Weekday, &rule.LocalStart, &rule.LocalEnd); err != nil {
			return nil, err
		}
		rule.LocalStart = clockText(rule.LocalStart)
		rule.LocalEnd = clockText(rule.LocalEnd)
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (s *Store) AddException(ctx context.Context, exception port.AvailabilityException) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO availability_exceptions (id, fitness_center_id, branch_id, trainer_user_id, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		exception.ID, exception.FitnessCenterID, exception.BranchID, exception.TrainerUserID, exception.StartsAt, exception.EndsAt)
	return err
}

func (s *Store) DeleteException(ctx context.Context, trainerID, exceptionID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM availability_exceptions WHERE id = $1 AND trainer_user_id = $2`, exceptionID, trainerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *Store) ListExceptions(ctx context.Context, centerID, trainerID string, from, to time.Time) ([]port.AvailabilityException, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, fitness_center_id::text, branch_id::text, trainer_user_id::text, starts_at, ends_at
		FROM availability_exceptions
		WHERE fitness_center_id = $1 AND trainer_user_id = $2 AND starts_at < $4 AND ends_at > $3
		ORDER BY starts_at`, centerID, trainerID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.AvailabilityException{}
	for rows.Next() {
		var exception port.AvailabilityException
		if err := rows.Scan(&exception.ID, &exception.FitnessCenterID, &exception.BranchID, &exception.TrainerUserID, &exception.StartsAt, &exception.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, exception)
	}
	return out, rows.Err()
}

func (s *Store) ListHeld(ctx context.Context, centerID, trainerID string, from, to time.Time) ([]port.HeldHour, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT starts_at, ends_at FROM bookings
		WHERE fitness_center_id = $1 AND trainer_user_id = $2
		  AND status IN ('requested', 'confirmed')
		  AND starts_at < $4 AND ends_at > $3`, centerID, trainerID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.HeldHour{}
	for rows.Next() {
		var hour port.HeldHour
		if err := rows.Scan(&hour.StartsAt, &hour.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, hour)
	}
	return out, rows.Err()
}

func (s *Store) CenterZone(ctx context.Context, centerID string) (string, error) {
	var zone string
	err := s.pool.QueryRow(ctx, `SELECT timezone FROM fitness_centers WHERE id = $1`, centerID).Scan(&zone)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", user.ErrNotFound
	}
	return zone, err
}

func (s *Store) AssignedTrainer(ctx context.Context, centerID, clientUserID string) (string, string, error) {
	var trainerID, branchID string
	err := s.pool.QueryRow(ctx, `
		SELECT trainer_user_id::text, branch_id::text FROM trainer_assignments
		WHERE fitness_center_id = $1 AND client_user_id = $2`, centerID, clientUserID).Scan(&trainerID, &branchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", user.ErrNotFound
	}
	return trainerID, branchID, err
}

func clockText(value string) string {
	if len(value) >= 5 {
		return value[:5]
	}
	return value
}

func (s *Store) InsertBooking(ctx context.Context, booking port.Booking) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bookings (id, fitness_center_id, branch_id, trainer_user_id, client_user_id, series_id, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, $7, $8, $9)`,
		booking.ID, booking.FitnessCenterID, booking.BranchID, booking.TrainerUserID, booking.ClientUserID,
		booking.SeriesID, booking.StartsAt, booking.EndsAt, booking.Status)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return schedule.ErrHourBooked
	}
	return err
}

func (s *Store) ListBookings(ctx context.Context, centerID, clientID, trainerID string, from, to time.Time) ([]port.Booking, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text, b.fitness_center_id::text, b.branch_id::text,
		       b.trainer_user_id::text, b.client_user_id::text,
		       trainer.display_name, client.display_name, COALESCE(b.series_id::text, ''),
		       b.starts_at, b.ends_at, b.status
		FROM bookings b
		JOIN users trainer ON trainer.id = b.trainer_user_id
		JOIN users client ON client.id = b.client_user_id
		WHERE b.fitness_center_id = $1
		  AND ($2 = '' OR b.client_user_id = $2::uuid)
		  AND ($3 = '' OR b.trainer_user_id = $3::uuid)
		  AND b.starts_at < $5 AND b.ends_at > $4
		ORDER BY b.starts_at`, centerID, clientID, trainerID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.Booking{}
	for rows.Next() {
		var booking port.Booking
		if err := rows.Scan(&booking.ID, &booking.FitnessCenterID, &booking.BranchID, &booking.TrainerUserID, &booking.ClientUserID, &booking.TrainerName, &booking.ClientName, &booking.SeriesID, &booking.StartsAt, &booking.EndsAt, &booking.Status); err != nil {
			return nil, err
		}
		out = append(out, booking)
	}
	return out, rows.Err()
}

func (s *Store) FindBooking(ctx context.Context, centerID, bookingID string) (port.Booking, error) {
	var booking port.Booking
	err := s.pool.QueryRow(ctx, `
		SELECT b.id::text, b.fitness_center_id::text, b.branch_id::text,
		       b.trainer_user_id::text, b.client_user_id::text,
		       trainer.display_name, client.display_name,
		       b.starts_at, b.ends_at, b.status
		FROM bookings b
		JOIN users trainer ON trainer.id = b.trainer_user_id
		JOIN users client ON client.id = b.client_user_id
		WHERE b.fitness_center_id = $1 AND b.id = $2`, centerID, bookingID).Scan(
		&booking.ID, &booking.FitnessCenterID, &booking.BranchID, &booking.TrainerUserID, &booking.ClientUserID,
		&booking.TrainerName, &booking.ClientName, &booking.StartsAt, &booking.EndsAt, &booking.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.Booking{}, user.ErrNotFound
	}
	return booking, err
}

func (s *Store) SetBookingStatus(ctx context.Context, centerID, bookingID, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE bookings SET status = $3, updated_at = now()
		WHERE fitness_center_id = $1 AND id = $2`, centerID, bookingID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *Store) SaveSeries(ctx context.Context, series port.Series, bookings []port.Booking) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO booking_series (id, fitness_center_id, branch_id, trainer_user_id, client_user_id, weekday, local_start, local_end, starts_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		series.ID, series.FitnessCenterID, series.BranchID, series.TrainerUserID, series.ClientUserID,
		series.Weekday, series.LocalStart, series.LocalEnd, series.StartsOn)
	if err != nil {
		return err
	}
	for _, booking := range bookings {
		_, err = tx.Exec(ctx, `
			INSERT INTO bookings (id, fitness_center_id, branch_id, trainer_user_id, client_user_id, series_id, starts_at, ends_at, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			booking.ID, booking.FitnessCenterID, booking.BranchID, booking.TrainerUserID, booking.ClientUserID,
			booking.SeriesID, booking.StartsAt, booking.EndsAt, booking.Status)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return schedule.ErrHourBooked
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) SeriesBookings(ctx context.Context, centerID, seriesID string) ([]port.Booking, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, fitness_center_id::text, branch_id::text, trainer_user_id::text, client_user_id::text,
		       '', '', COALESCE(series_id::text, ''), starts_at, ends_at, status
		FROM bookings
		WHERE fitness_center_id = $1 AND series_id = $2
		ORDER BY starts_at`, centerID, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.Booking{}
	for rows.Next() {
		var booking port.Booking
		if err := rows.Scan(&booking.ID, &booking.FitnessCenterID, &booking.BranchID, &booking.TrainerUserID, &booking.ClientUserID, &booking.TrainerName, &booking.ClientName, &booking.SeriesID, &booking.StartsAt, &booking.EndsAt, &booking.Status); err != nil {
			return nil, err
		}
		out = append(out, booking)
	}
	return out, rows.Err()
}

func (s *Store) MoveBooking(ctx context.Context, centerID, bookingID string, start, end time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE bookings SET starts_at = $3, ends_at = $4, updated_at = now()
		WHERE fitness_center_id = $1 AND id = $2`, centerID, bookingID, start, end)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
		return schedule.ErrHourBooked
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *Store) SaveSeriesSkip(ctx context.Context, centerID, branchID, seriesID, occurrence string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO booking_series_exceptions (id, series_id, fitness_center_id, branch_id, occurrence_on, kind)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 'skip')`, seriesID, centerID, branchID, occurrence)
	return err
}

func (s *Store) AddNotification(ctx context.Context, notice port.Notification) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notifications (id, fitness_center_id, user_id, kind)
		VALUES ($1, $2, $3, $4)`,
		notice.ID, notice.FitnessCenterID, notice.UserID, notice.Kind+":"+notice.BookingID)
	return err
}

func (s *Store) ListNotifications(ctx context.Context, centerID, userID string) ([]port.Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, fitness_center_id::text, user_id::text, kind, read_at, created_at
		FROM notifications
		WHERE fitness_center_id = $1 AND user_id = $2
		ORDER BY created_at DESC`, centerID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.Notification{}
	for rows.Next() {
		var notice port.Notification
		var kind string
		var readAt *time.Time
		if err := rows.Scan(&notice.ID, &notice.FitnessCenterID, &notice.UserID, &kind, &readAt, &notice.CreatedAt); err != nil {
			return nil, err
		}
		notice.Kind, notice.BookingID = splitNotice(kind)
		notice.Read = readAt != nil
		out = append(out, notice)
	}
	return out, rows.Err()
}

func (s *Store) ReadNotification(ctx context.Context, centerID, userID, noticeID string) (port.Notification, error) {
	var notice port.Notification
	var kind string
	err := s.pool.QueryRow(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE id = $1 AND fitness_center_id = $2 AND user_id = $3
		RETURNING id::text, fitness_center_id::text, user_id::text, kind, created_at`,
		noticeID, centerID, userID).Scan(&notice.ID, &notice.FitnessCenterID, &notice.UserID, &kind, &notice.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.Notification{}, user.ErrNotFound
	}
	if err != nil {
		return port.Notification{}, err
	}
	notice.Kind, notice.BookingID = splitNotice(kind)
	notice.Read = true
	return notice, nil
}

func (s *Store) SaveLog(ctx context.Context, log port.SessionLog) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branchID, trainerID string
	err = tx.QueryRow(ctx, `
		SELECT branch_id::text, trainer_user_id::text FROM bookings
		WHERE id = $1 AND fitness_center_id = $2`, log.BookingID, log.FitnessCenterID).Scan(&branchID, &trainerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO session_logs (id, booking_id, fitness_center_id, branch_id, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (booking_id) DO UPDATE SET note = EXCLUDED.note, updated_at = now()`,
		log.ID, log.BookingID, log.FitnessCenterID, branchID, log.Note)
	if err != nil {
		return err
	}
	var logID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM session_logs WHERE booking_id = $1`, log.BookingID).Scan(&logID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM session_log_sets WHERE session_log_id = $1`, logID); err != nil {
		return err
	}
	for i, set := range log.Sets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO session_log_sets (id, session_log_id, fitness_center_id, branch_id, position, exercise_name, reps, load, unit)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, 'kg')`,
			logID, log.FitnessCenterID, branchID, i, set.ExerciseName, set.Reps, set.Load); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'completed', updated_at = now() WHERE id = $1`, log.BookingID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (id, fitness_center_id, actor_user_id, action, entity_type, entity_id)
		VALUES (gen_random_uuid(), $1, $2, 'session_log', 'session_log', $3)`,
		log.FitnessCenterID, trainerID, logID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FindLog(ctx context.Context, centerID, bookingID string) (port.SessionLog, error) {
	var log port.SessionLog
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, booking_id::text, fitness_center_id::text, branch_id::text, note, updated_at
		FROM session_logs WHERE fitness_center_id = $1 AND booking_id = $2`, centerID, bookingID).Scan(
		&log.ID, &log.BookingID, &log.FitnessCenterID, &log.BranchID, &log.Note, &log.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.SessionLog{}, user.ErrNotFound
	}
	if err != nil {
		return port.SessionLog{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT exercise_name, reps, load, unit FROM session_log_sets
		WHERE session_log_id = $1 ORDER BY position`, log.ID)
	if err != nil {
		return port.SessionLog{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var set port.SessionSet
		if err := rows.Scan(&set.ExerciseName, &set.Reps, &set.Load, &set.Unit); err != nil {
			return port.SessionLog{}, err
		}
		log.Sets = append(log.Sets, set)
	}
	return log, rows.Err()
}

func splitNotice(kind string) (string, string) {
	parts := strings.SplitN(kind, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return kind, ""
}
