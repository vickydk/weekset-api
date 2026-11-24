package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// Store is the Postgres account and token store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a pool. The caller closes it.
func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Create(ctx context.Context, account user.User) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, display_name, account_type)
		VALUES ($1, $2, $3, $4, $5)`,
		account.ID, account.Email, account.PasswordHash, account.DisplayName, account.AccountType)
	if isUnique(err) {
		return user.ErrEmailInUse
	}
	return err
}

func (s *Store) FindByEmail(ctx context.Context, email string) (user.User, error) {
	return s.find(ctx, `SELECT id, email, password_hash, display_name, account_type FROM users WHERE lower(email) = lower($1)`, email)
}

func (s *Store) FindByID(ctx context.Context, id string) (user.User, error) {
	return s.find(ctx, `SELECT id, email, password_hash, display_name, account_type FROM users WHERE id = $1`, id)
}

func (s *Store) find(ctx context.Context, sql string, arg any) (user.User, error) {
	var account user.User
	err := s.pool.QueryRow(ctx, sql, arg).Scan(&account.ID, &account.Email, &account.PasswordHash, &account.DisplayName, &account.AccountType)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, user.ErrNotFound
	}
	return account, err
}

func (s *Store) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, passwordHash)
	return err
}

func (s *Store) Memberships(ctx context.Context, userID string) ([]port.MembershipView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.role, m.status, m.fitness_center_id, c.name,
		       COALESCE(m.branch_id::text, ''), COALESCE(b.name, ''),
		       COALESCE(ta.trainer_user_id::text, '')
		FROM memberships m
		JOIN fitness_centers c ON c.id = m.fitness_center_id
		LEFT JOIN branches b ON b.id = m.branch_id
		LEFT JOIN trainer_assignments ta
		  ON ta.fitness_center_id = m.fitness_center_id AND ta.client_user_id = m.user_id
		WHERE m.user_id = $1
		ORDER BY c.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []port.MembershipView
	var membershipIDs []string
	for rows.Next() {
		var id string
		var row port.MembershipView
		if err := rows.Scan(&id, &row.Role, &row.Status, &row.FitnessCenterID, &row.CenterName, &row.BranchID, &row.BranchName, &row.TrainerUserID); err != nil {
			return nil, err
		}
		row.Branches = []port.BranchView{}
		out = append(out, row)
		membershipIDs = append(membershipIDs, id)
	}
	if out == nil {
		out = []port.MembershipView{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, id := range membershipIDs {
		var branches []port.BranchView
		var err error
		if out[i].Role == "center_admin" {
			listed, listErr := s.ListBranches(ctx, out[i].FitnessCenterID)
			err = listErr
			branches = make([]port.BranchView, 0, len(listed))
			for _, branch := range listed {
				branches = append(branches, port.BranchView{ID: branch.ID, Name: branch.Name})
			}
		} else {
			branches, err = s.managerBranches(ctx, id)
		}
		if err != nil {
			return nil, err
		}
		out[i].Branches = branches
	}
	return out, nil
}

func (s *Store) managerBranches(ctx context.Context, membershipID string) ([]port.BranchView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.name
		FROM manager_branches mb
		JOIN branches b ON b.id = mb.branch_id
		WHERE mb.membership_id = $1
		ORDER BY b.name`, membershipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []port.BranchView{}
	for rows.Next() {
		var branch port.BranchView
		if err := rows.Scan(&branch.ID, &branch.Name); err != nil {
			return nil, err
		}
		out = append(out, branch)
	}
	return out, rows.Err()
}

func (s *Store) SaveRefresh(ctx context.Context, token port.RefreshToken) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, family_id)
		VALUES ($1, $2, $3, $4, $5)`, token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.FamilyID)
	return err
}

func (s *Store) FindRefresh(ctx context.Context, tokenHash string) (port.RefreshToken, error) {
	var token port.RefreshToken
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, family_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&token.ID, &token.UserID, &token.FamilyID, &token.TokenHash, &token.ExpiresAt, &token.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.RefreshToken{}, user.ErrNotFound
	}
	return token, err
}

func (s *Store) RevokeRefresh(ctx context.Context, id string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`, id, at)
	return err
}

func (s *Store) RevokeFamily(ctx context.Context, familyID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = $2 WHERE family_id = $1 AND revoked_at IS NULL`, familyID, at)
	return err
}

func (s *Store) RevokeUserRefresh(ctx context.Context, userID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, at)
	return err
}

func (s *Store) SaveReset(ctx context.Context, reset port.PasswordReset) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO password_resets (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)`, reset.ID, reset.UserID, reset.TokenHash, reset.ExpiresAt)
	return err
}

func (s *Store) FindReset(ctx context.Context, tokenHash string) (port.PasswordReset, error) {
	var reset port.PasswordReset
	err := s.pool.QueryRow(ctx, `SELECT id, user_id, token_hash, expires_at, used_at FROM password_resets WHERE token_hash = $1`, tokenHash).
		Scan(&reset.ID, &reset.UserID, &reset.TokenHash, &reset.ExpiresAt, &reset.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.PasswordReset{}, user.ErrNotFound
	}
	return reset, err
}

func (s *Store) MarkResetUsed(ctx context.Context, id string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE password_resets SET used_at = $2 WHERE id = $1`, id, at)
	return err
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
