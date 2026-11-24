package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func (s *Store) FindInvite(ctx context.Context, codeHash string) (port.Invite, error) {
	var invite port.Invite
	var expires time.Time
	var accepted *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, fitness_center_id::text, email, role, branch_ids::text[], code_hash,
		       expires_at, accepted_at
		FROM invites WHERE code_hash = $1`, codeHash).
		Scan(&invite.ID, &invite.FitnessCenterID, &invite.Email, &invite.Role, &invite.BranchIDs, &invite.CodeHash, &expires, &accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.Invite{}, user.ErrNotFound
	}
	if err != nil {
		return port.Invite{}, err
	}
	invite.ExpiresAt = expires.UTC().Format(time.RFC3339)
	invite.Accepted = accepted != nil
	return invite, nil
}

func (s *Store) AcceptInvite(ctx context.Context, inviteID string, draft port.MembershipDraft) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var accepted bool
	err = tx.QueryRow(ctx, `SELECT accepted_at IS NOT NULL FROM invites WHERE id = $1 FOR UPDATE`, inviteID).Scan(&accepted)
	if errors.Is(err, pgx.ErrNoRows) || accepted {
		return user.ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO memberships (id, user_id, fitness_center_id, role, status, branch_id)
		VALUES ($1, $2, $3, $4, 'active', NULLIF($5, '')::uuid)`,
		draft.ID, draft.UserID, draft.FitnessCenterID, draft.Role, draft.BranchID)
	if err != nil {
		return membershipConflict(err, draft.Role)
	}
	for _, branchID := range draft.ManagerBranchIDs {
		_, err = tx.Exec(ctx, `INSERT INTO manager_branches (membership_id, fitness_center_id, branch_id) VALUES ($1, $2, $3)`,
			draft.ID, draft.FitnessCenterID, branchID)
		if err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE invites SET accepted_at = now() WHERE id = $1 AND accepted_at IS NULL`, inviteID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (s *Store) ListPeople(ctx context.Context, centerID, branchID string, includeManagers bool) ([]port.Person, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id::text, u.display_name, u.email, m.role, m.status,
		       COALESCE(m.branch_id::text, ''), COALESCE(b.name, ''),
		       COALESCE(tu.display_name, '')
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		LEFT JOIN branches b ON b.id = m.branch_id
		LEFT JOIN trainer_assignments ta
		  ON ta.fitness_center_id = m.fitness_center_id AND ta.client_user_id = m.user_id
		LEFT JOIN users tu ON tu.id = ta.trainer_user_id
		WHERE m.fitness_center_id = $1
		  AND m.role IN ('client', 'trainer')
		  AND ($2 = '' OR m.branch_id::text = $2)
		ORDER BY m.role, u.display_name`, centerID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.Person{}
	for rows.Next() {
		var person port.Person
		if err := rows.Scan(&person.UserID, &person.DisplayName, &person.Email, &person.Role, &person.Status, &person.BranchID, &person.BranchName, &person.TrainerName); err != nil {
			return nil, err
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !includeManagers {
		return out, nil
	}
	managers, err := s.pool.Query(ctx, `
		SELECT u.id::text, u.display_name, u.email, m.status, b.id::text, b.name
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN manager_branches mb ON mb.membership_id = m.id
		JOIN branches b ON b.id = mb.branch_id
		WHERE m.fitness_center_id = $1 AND m.role = 'manager'
		  AND ($2 = '' OR b.id::text = $2)
		ORDER BY u.display_name, b.name`, centerID, branchID)
	if err != nil {
		return nil, err
	}
	defer managers.Close()
	for managers.Next() {
		var person port.Person
		person.Role = "manager"
		if err := managers.Scan(&person.UserID, &person.DisplayName, &person.Email, &person.Status, &person.BranchID, &person.BranchName); err != nil {
			return nil, err
		}
		out = append(out, person)
	}
	return out, managers.Err()
}

func (s *Store) ListOpenInvites(ctx context.Context, centerID string) ([]port.Invite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, fitness_center_id::text, email, role, branch_ids::text[], code_hash, expires_at
		FROM invites
		WHERE fitness_center_id = $1 AND accepted_at IS NULL
		ORDER BY created_at`, centerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.Invite{}
	for rows.Next() {
		var invite port.Invite
		var expires time.Time
		if err := rows.Scan(&invite.ID, &invite.FitnessCenterID, &invite.Email, &invite.Role, &invite.BranchIDs, &invite.CodeHash, &expires); err != nil {
			return nil, err
		}
		invite.ExpiresAt = expires.UTC().Format(time.RFC3339)
		out = append(out, invite)
	}
	return out, rows.Err()
}

func (s *Store) FindMember(ctx context.Context, centerID, userID string) (port.Member, error) {
	var member port.Member
	err := s.pool.QueryRow(ctx, `
		SELECT m.id::text, m.user_id::text, m.role, m.status, COALESCE(m.branch_id::text, ''), u.display_name, u.email
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.fitness_center_id = $1 AND m.user_id = $2`, centerID, userID).
		Scan(&member.MembershipID, &member.UserID, &member.Role, &member.Status, &member.BranchID, &member.DisplayName, &member.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.Member{}, user.ErrNotFound
	}
	return member, err
}

func (s *Store) SetMembershipStatus(ctx context.Context, centerID, userID, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE memberships SET status = $3, updated_at = now()
		WHERE fitness_center_id = $1 AND user_id = $2`, centerID, userID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteMembership(ctx context.Context, centerID, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM memberships WHERE fitness_center_id = $1 AND user_id = $2`, centerID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *Store) AssignTrainer(ctx context.Context, draft port.Assignment) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM trainer_assignments WHERE fitness_center_id = $1 AND client_user_id = $2`, draft.FitnessCenterID, draft.ClientUserID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO trainer_assignments (
			id, fitness_center_id, branch_id, client_user_id, trainer_user_id, client_membership_id, trainer_membership_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		draft.ID, draft.FitnessCenterID, draft.BranchID, draft.ClientUserID, draft.TrainerUserID, draft.ClientMembershipID, draft.TrainerMembershipID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CountTrainerRules(ctx context.Context, centerID, trainerUserID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM availability_rules
		WHERE fitness_center_id = $1 AND trainer_user_id = $2`, centerID, trainerUserID).Scan(&count)
	return count, err
}

func membershipConflict(err error, role string) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	if pgErr.ConstraintName == "memberships_one_staff_membership_idx" {
		switch role {
		case string(membership.RoleTrainer):
			return membership.ErrTrainerHasBranch
		case string(membership.RoleManager):
			return membership.ErrManagerHasCenter
		default:
			return membership.ErrCenterAdminHasCenter
		}
	}
	return membership.ErrAlreadyInCenter
}
