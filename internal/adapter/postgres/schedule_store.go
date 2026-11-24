package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

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
