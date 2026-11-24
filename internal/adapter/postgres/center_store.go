package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func (s *Store) CreateCenter(ctx context.Context, draft port.CenterDraft) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO users (id, email, password_hash, display_name, account_type)
		VALUES ($1, $2, $3, $4, 'center')`, draft.UserID, draft.Email, draft.PasswordHash, draft.DisplayName)
	if isUnique(err) {
		return user.ErrEmailInUse
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO fitness_centers (id, name, timezone) VALUES ($1, $2, $3)`,
		draft.CenterID, draft.CenterName, draft.Timezone)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO branches (id, fitness_center_id, name) VALUES ($1, $2, $3)`,
		draft.BranchID, draft.CenterID, draft.BranchName)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status)
		VALUES ($1, $2, $3, 'center_admin', 'active')`, draft.MembershipID, draft.UserID, draft.CenterID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AddBranch(ctx context.Context, centerID, branchID, name string) (port.Branch, error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO branches (id, fitness_center_id, name) VALUES ($1, $2, $3)`, branchID, centerID, name)
	if err != nil {
		return port.Branch{}, err
	}
	return port.Branch{ID: branchID, FitnessCenterID: centerID, Name: name}, nil
}

func (s *Store) ListBranches(ctx context.Context, centerID string) ([]port.Branch, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, fitness_center_id, name FROM branches WHERE fitness_center_id = $1 ORDER BY name`, centerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.Branch{}
	for rows.Next() {
		var branch port.Branch
		if err := rows.Scan(&branch.ID, &branch.FitnessCenterID, &branch.Name); err != nil {
			return nil, err
		}
		out = append(out, branch)
	}
	return out, rows.Err()
}

func (s *Store) RenameBranch(ctx context.Context, centerID, branchID, name string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE branches SET name = $3, updated_at = now() WHERE id = $1 AND fitness_center_id = $2`, branchID, centerID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *Store) RenameCenter(ctx context.Context, centerID, name, timezone string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE fitness_centers
		SET name = CASE WHEN $2 = '' THEN name ELSE $2 END,
		    timezone = CASE WHEN $3 = '' THEN timezone ELSE $3 END,
		    updated_at = now()
		WHERE id = $1`, centerID, name, timezone)
	return err
}

func (s *Store) SaveInvite(ctx context.Context, invite port.Invite) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO invites (id, fitness_center_id, email, role, branch_ids, code_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5::uuid[], $6, $7)`,
		invite.ID, invite.FitnessCenterID, invite.Email, invite.Role, uuidArray(invite.BranchIDs), invite.CodeHash, invite.ExpiresAt)
	return err
}

func (s *Store) ReplaceManagerBranches(ctx context.Context, centerID, userID string, branchIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var membershipID string
	err = tx.QueryRow(ctx, `SELECT id FROM memberships WHERE user_id = $1 AND fitness_center_id = $2 AND role = 'manager'`, userID, centerID).Scan(&membershipID)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM manager_branches WHERE membership_id = $1`, membershipID); err != nil {
		return err
	}
	for _, branchID := range branchIDs {
		_, err := tx.Exec(ctx, `INSERT INTO manager_branches (membership_id, fitness_center_id, branch_id) VALUES ($1, $2, $3)`,
			membershipID, centerID, branchID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) BranchBelongs(ctx context.Context, centerID, branchID string) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1 AND fitness_center_id = $2)`, branchID, centerID).Scan(&found)
	return found, err
}

func uuidArray(ids []string) string {
	return "{" + strings.Join(ids, ",") + "}"
}
