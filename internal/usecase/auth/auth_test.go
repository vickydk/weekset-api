package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vickydk/weekset-api/internal/adapter/memory"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/password"
	"github.com/vickydk/weekset-api/internal/platform/token"
)

func TestRegisterReturnsTokensAndNoMembership(t *testing.T) {
	svc, _ := newService()
	session, err := svc.Register(context.Background(), RegisterInput{
		Email:       "Maya@Harbor.club",
		Password:    "long-enough",
		DisplayName: "Maya",
		AccountType: user.AccountClient,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, session.AccessToken)
	assert.NotEmpty(t, session.RefreshToken)
	assert.Equal(t, 900, session.ExpiresIn)
	assert.Equal(t, "maya@harbor.club", session.Profile.Email)
	assert.Empty(t, session.Profile.Memberships)
	assert.Equal(t, user.AccountClient, session.Profile.AccountType)
}

func TestRegisterRejectsACenterAccount(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:       "ada@harbor.club",
		Password:    "long-enough",
		DisplayName: "Ada",
		AccountType: user.AccountCenter,
	})
	require.Error(t, err)
}

func TestDuplicateEmailConflicts(t *testing.T) {
	svc, _ := newService()
	in := RegisterInput{Email: "maya@harbor.club", Password: "long-enough", DisplayName: "Maya", AccountType: user.AccountTrainer}
	_, err := svc.Register(context.Background(), in)
	require.NoError(t, err)
	_, err = svc.Register(context.Background(), in)
	assert.ErrorIs(t, err, user.ErrEmailInUse)
}

func TestLoginHidesUnknownAndWrongPassword(t *testing.T) {
	svc, _ := newService()
	_, err := svc.Register(context.Background(), RegisterInput{
		Email: "maya@harbor.club", Password: "long-enough", DisplayName: "Maya", AccountType: user.AccountManager,
	})
	require.NoError(t, err)

	_, unknown := svc.Login(context.Background(), "missing@harbor.club", "long-enough")
	_, wrong := svc.Login(context.Background(), "maya@harbor.club", "not-the-password")
	assert.ErrorIs(t, unknown, user.ErrUnauthorized)
	assert.ErrorIs(t, wrong, user.ErrUnauthorized)
	assert.Equal(t, unknown.Error(), wrong.Error())
}

func TestRefreshReuseRevokesTheFamily(t *testing.T) {
	svc, _ := newService()
	first, err := svc.Register(context.Background(), RegisterInput{
		Email: "maya@harbor.club", Password: "long-enough", DisplayName: "Maya", AccountType: user.AccountClient,
	})
	require.NoError(t, err)
	second, err := svc.Refresh(context.Background(), first.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)

	_, err = svc.Refresh(context.Background(), first.RefreshToken)
	assert.ErrorIs(t, err, user.ErrUnauthorized)
	_, err = svc.Refresh(context.Background(), second.RefreshToken)
	assert.ErrorIs(t, err, user.ErrUnauthorized)
}

func TestForgotDoesNotRevealAMissingEmail(t *testing.T) {
	svc, _ := newService()
	require.NoError(t, svc.Forgot(context.Background(), "missing@harbor.club"))
}

func TestResetRevokesRefreshTokens(t *testing.T) {
	svc, store := newService()
	session, err := svc.Register(context.Background(), RegisterInput{
		Email: "maya@harbor.club", Password: "long-enough", DisplayName: "Maya", AccountType: user.AccountClient,
	})
	require.NoError(t, err)
	require.NoError(t, svc.Forgot(context.Background(), "maya@harbor.club"))
	require.NoError(t, svc.ResetPassword(context.Background(), store.LastResetToken(), "another-long"))
	_, err = svc.Refresh(context.Background(), session.RefreshToken)
	assert.ErrorIs(t, err, user.ErrUnauthorized)
	_, err = svc.Login(context.Background(), "maya@harbor.club", "another-long")
	assert.NoError(t, err)
}

func newService() (*Service, *memory.Store) {
	store := memory.NewStore()
	hasher := password.Hasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 16}
	svc := &Service{
		Users:  store,
		Tokens: store,
		Resets: store,
		Mailer: store,
		Hasher: hasher,
		Issuer: token.Issuer{Secret: []byte("test-secret"), TTL: accessTTL},
		Now:    func() time.Time { return time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC) },
	}
	return svc, store
}
