package center

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vickydk/weekset-api/internal/adapter/memory"
	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/password"
	"github.com/vickydk/weekset-api/internal/platform/token"
	"github.com/vickydk/weekset-api/internal/port"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
)

func TestOpenCreatesTheCenterAndAnActiveAdmin(t *testing.T) {
	svc, _ := newCenter()
	session, err := svc.Open(context.Background(), OpenInput{
		Email: "Ada@Harbor.club", Password: "long-enough", DisplayName: "Ada",
		CenterName: "Harbor Club", BranchName: "Downtown", Timezone: "Asia/Jakarta",
	})
	require.NoError(t, err)
	require.Len(t, session.Profile.Memberships, 1)
	row := session.Profile.Memberships[0]
	assert.Equal(t, "center_admin", row.Role)
	assert.Equal(t, "active", row.Status)
	assert.Equal(t, "Harbor Club", row.CenterName)
	assert.Empty(t, row.BranchID)
	require.Len(t, row.Branches, 1)
	assert.Equal(t, "Downtown", row.Branches[0].Name)
}

func TestManagerCannotAddABranch(t *testing.T) {
	svc, store := newCenter()
	store.Create(context.Background(), user.User{
		ID: "manager-1", Email: "lee@harbor.club", PasswordHash: "x", DisplayName: "Lee", AccountType: user.AccountManager,
	})
	store.PutMemberships("manager-1", []port.MembershipView{{
		Role: "manager", Status: "active", FitnessCenterID: "center-1", CenterName: "Harbor",
		Branches: []port.BranchView{{ID: "branch-1", Name: "Downtown"}},
	}})
	_, err := svc.AddBranch(context.Background(), "manager-1", "Kemang")
	var denied *membership.AccessError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, membership.CodeForbidden, denied.Code)
}

func TestInviteRejectsTheWrongAccount(t *testing.T) {
	svc, store := newCenter()
	session, err := svc.Open(context.Background(), OpenInput{
		Email: "ada@harbor.club", Password: "long-enough", DisplayName: "Ada",
		CenterName: "Harbor Club", BranchName: "Downtown", Timezone: "Asia/Jakarta",
	})
	require.NoError(t, err)
	store.Create(context.Background(), user.User{
		ID: "client-1", Email: "maya@harbor.club", PasswordHash: "x", DisplayName: "Maya", AccountType: user.AccountClient,
	})
	_, _, _, err = svc.InviteManager(context.Background(), session.Profile.ID, "maya@harbor.club", []string{session.Profile.Memberships[0].Branches[0].ID})
	assert.EqualError(t, err, "account is not waiting as a manager")

	store.Create(context.Background(), user.User{
		ID: "manager-1", Email: "lee@harbor.club", PasswordHash: "x", DisplayName: "Lee", AccountType: user.AccountManager,
	})
	store.PutMemberships("manager-1", []port.MembershipView{{
		Role: "manager", Status: "active", FitnessCenterID: "other", CenterName: "Other",
	}})
	_, _, _, err = svc.InviteManager(context.Background(), session.Profile.ID, "lee@harbor.club", []string{session.Profile.Memberships[0].Branches[0].ID})
	assert.EqualError(t, err, "manager already belongs to a center")
}

func TestInviteShowsACodeOnce(t *testing.T) {
	svc, store := newCenter()
	session, err := svc.Open(context.Background(), OpenInput{
		Email: "ada@harbor.club", Password: "long-enough", DisplayName: "Ada",
		CenterName: "Harbor Club", BranchName: "Downtown", Timezone: "Asia/Jakarta",
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "manager-1", Email: "lee@harbor.club", PasswordHash: "x", DisplayName: "Lee", AccountType: user.AccountManager,
	}))
	id, code, expires, err := svc.InviteManager(context.Background(), session.Profile.ID, "lee@harbor.club", []string{session.Profile.Memberships[0].Branches[0].ID})
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.NotEmpty(t, code)
	assert.True(t, expires.After(time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)))
}

func newCenter() (*Service, *memory.Store) {
	store := memory.NewStore()
	hasher := password.Hasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 16}
	authSvc := &authuc.Service{
		Users: store, Tokens: store, Resets: store, Mailer: store, Hasher: hasher,
		Issuer: token.Issuer{Secret: []byte("test-secret"), TTL: 15 * time.Minute},
		Now:    func() time.Time { return time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC) },
	}
	return &Service{
		Centers: store,
		Users:   store,
		Auth:    authSvc,
		Hasher:  hasher,
		Now:     authSvc.Now,
	}, store
}
