//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	migrations := migrationDir(t)
	migrator, err := migrate.New("file://"+migrations, url)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	require.NoError(t, downAll(migrator))
	require.NoError(t, migrator.Up())
	t.Cleanup(func() {
		require.NoError(t, downAll(migrator))
	})

	conn, err := pgx.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close(ctx))
	})

	centerA := "00000000-0000-4000-8000-0000000000a1"
	centerB := "00000000-0000-4000-8000-0000000000a2"
	branchA := "00000000-0000-4000-8000-0000000000b1"
	branchB := "00000000-0000-4000-8000-0000000000b2"
	clientID := "00000000-0000-4000-8000-0000000000c1"
	trainerID := "00000000-0000-4000-8000-0000000000d1"
	managerID := "00000000-0000-4000-8000-0000000000e1"

	mustExec(t, conn, `INSERT INTO fitness_centers (id, name, timezone) VALUES ($1, 'Harbor', 'Asia/Jakarta'), ($2, 'North', 'Asia/Jakarta')`, centerA, centerB)
	mustExec(t, conn, `INSERT INTO branches (id, fitness_center_id, name) VALUES ($1, $2, 'Downtown'), ($3, $4, 'North')`, branchA, centerA, branchB, centerB)
	mustExec(t, conn, `INSERT INTO users (id, email, password_hash, display_name, account_type) VALUES
		($1, 'maya@harbor.club', 'hash', 'Maya', 'client'),
		($2, 'alex@harbor.club', 'hash', 'Alex', 'trainer'),
		($3, 'lee@harbor.club', 'hash', 'Lee', 'manager')`, clientID, trainerID, managerID)

	clientA := "00000000-0000-4000-8000-000000000101"
	clientB := "00000000-0000-4000-8000-000000000102"
	mustExec(t, conn, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status, branch_id) VALUES
		($1, $2, $3, 'client', 'active', $4),
		($5, $2, $6, 'client', 'active', $7)`,
		clientA, clientID, centerA, branchA, clientB, centerB, branchB)

	var centers int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE user_id = $1 AND role = 'client'`, clientID).Scan(&centers))
	assert.Equal(t, 2, centers)

	suspended := "00000000-0000-4000-8000-000000000103"
	otherClient := "00000000-0000-4000-8000-0000000000c2"
	mustExec(t, conn, `INSERT INTO users (id, email, password_hash, display_name, account_type) VALUES ($1, 'sam@harbor.club', 'hash', 'Sam', 'client')`, otherClient)
	mustExec(t, conn, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status, branch_id) VALUES ($1, $2, $3, 'client', 'suspended', $4)`, suspended, otherClient, centerA, branchA)
	var status string
	require.NoError(t, conn.QueryRow(ctx, `SELECT status FROM memberships WHERE id = $1`, suspended).Scan(&status))
	assert.Equal(t, "suspended", status)

	_, err = conn.Exec(ctx, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status) VALUES ($1, $2, $3, 'trainer', 'active')`,
		"00000000-0000-4000-8000-000000000201", trainerID, centerA)
	assert.Equal(t, "23514", pgCode(t, err))

	trainerMembership := "00000000-0000-4000-8000-000000000202"
	mustExec(t, conn, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status, branch_id) VALUES ($1, $2, $3, 'trainer', 'active', $4)`,
		trainerMembership, trainerID, centerA, branchA)
	_, err = conn.Exec(ctx, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status, branch_id) VALUES ($1, $2, $3, 'trainer', 'active', $4)`,
		"00000000-0000-4000-8000-000000000203", trainerID, centerB, branchB)
	assert.Equal(t, "23505", pgCode(t, err))

	managerMembership := "00000000-0000-4000-8000-000000000301"
	mustExec(t, conn, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status) VALUES ($1, $2, $3, 'manager', 'active')`,
		managerMembership, managerID, centerA)
	_, err = conn.Exec(ctx, `INSERT INTO memberships (id, user_id, fitness_center_id, role, status) VALUES ($1, $2, $3, 'manager', 'active')`,
		"00000000-0000-4000-8000-000000000302", managerID, centerB)
	assert.Equal(t, "23505", pgCode(t, err))

	mustExec(t, conn, `INSERT INTO bookings (id, fitness_center_id, branch_id, trainer_user_id, client_user_id, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, $5, '2026-11-24T07:00:00Z', '2026-11-24T08:00:00Z', 'requested')`,
		"00000000-0000-4000-8000-000000000401", centerA, branchA, trainerID, clientID)
	_, err = conn.Exec(ctx, `INSERT INTO bookings (id, fitness_center_id, branch_id, trainer_user_id, client_user_id, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, $5, '2026-11-24T07:30:00Z', '2026-11-24T08:30:00Z', 'confirmed')`,
		"00000000-0000-4000-8000-000000000402", centerA, branchA, trainerID, clientID)
	assert.Equal(t, "23P01", pgCode(t, err))
	mustExec(t, conn, `INSERT INTO bookings (id, fitness_center_id, branch_id, trainer_user_id, client_user_id, starts_at, ends_at, status)
		VALUES ($1, $2, $3, $4, $5, '2026-11-24T07:30:00Z', '2026-11-24T08:30:00Z', 'declined')`,
		"00000000-0000-4000-8000-000000000403", centerA, branchA, trainerID, clientID)

	mustExec(t, conn, `INSERT INTO trainer_assignments (id, fitness_center_id, branch_id, client_user_id, trainer_user_id, client_membership_id, trainer_membership_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"00000000-0000-4000-8000-000000000501", centerA, branchA, clientID, trainerID, clientA, trainerMembership)
	mustExec(t, conn, `INSERT INTO session_logs (id, booking_id, fitness_center_id, branch_id, note)
		VALUES ($1, $2, $3, $4, 'hips felt tight')`,
		"00000000-0000-4000-8000-000000000601", "00000000-0000-4000-8000-000000000401", centerA, branchA)

	mustExec(t, conn, `DELETE FROM memberships WHERE id = $1`, clientA)

	var assignments int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM trainer_assignments WHERE client_user_id = $1`, clientID).Scan(&assignments))
	assert.Equal(t, 0, assignments)
	var logs int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM session_logs WHERE id = $1`, "00000000-0000-4000-8000-000000000601").Scan(&logs))
	assert.Equal(t, 1, logs)
	var bookings int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE id = $1`, "00000000-0000-4000-8000-000000000401").Scan(&bookings))
	assert.Equal(t, 1, bookings)

	var exercises int
	require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM exercises`).Scan(&exercises))
	assert.Equal(t, 12, exercises)

	require.NoError(t, downAll(migrator))
	var usersTable *string
	require.NoError(t, conn.QueryRow(ctx, `SELECT to_regclass('public.users')::text`).Scan(&usersTable))
	assert.Nil(t, usersTable)
}

func downAll(m *migrate.Migrate) error {
	err := m.Down()
	if err == nil || errors.Is(err, migrate.ErrNoChange) || errors.Is(err, migrate.ErrNilVersion) {
		return nil
	}
	return err
}

func mustExec(t *testing.T, conn *pgx.Conn, sql string, args ...any) {
	t.Helper()
	_, err := conn.Exec(context.Background(), sql, args...)
	require.NoError(t, err)
}

func pgCode(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	return pgErr.Code
}

func migrationDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations"))
}
