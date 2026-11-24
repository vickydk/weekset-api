package home

import (
	"context"
	"encoding/json"
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
	centeruc "github.com/vickydk/weekset-api/internal/usecase/center"
)

func TestANewCenterAsksForAManager(t *testing.T) {
	svc, centers := newHome()
	session, err := centers.Open(context.Background(), centeruc.OpenInput{
		Email: "ada@harbor.club", Password: "long-enough", DisplayName: "Ada",
		CenterName: "Harbor Club", BranchName: "Downtown", Timezone: "Asia/Jakarta",
	})
	require.NoError(t, err)
	card, err := svc.Home(context.Background(), session.Profile.ID, membership.Headers{})
	require.NoError(t, err)
	assert.Equal(t, "Which branch needs an owner?", card.Question)
	assert.Equal(t, "Invite a manager for Downtown.", card.EmptyTitle)
	assert.Equal(t, "Invite a manager", card.PrimaryAction)
	raw, err := json.Marshal(map[string]any{
		"question": card.Question, "empty_title": card.EmptyTitle, "sessions": card.Sessions, "branches": card.Branches,
	})
	require.NoError(t, err)
	body := string(raw)
	assert.NotContains(t, body, "note")
	assert.NotContains(t, body, "sets")
	assert.NotContains(t, body, "reps")
	assert.NotContains(t, body, "load")
}

func TestASuspendedMembershipCannotOpenHome(t *testing.T) {
	svc, _ := newHome()
	store := svc.Centers.(*memory.Store)
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "client-1", Email: "maya@harbor.club", PasswordHash: "x", DisplayName: "Maya", AccountType: user.AccountClient,
	}))
	store.PutMemberships("client-1", []port.MembershipView{{
		Role: "client", Status: "suspended", FitnessCenterID: "center-1", CenterName: "Harbor Club",
	}})
	_, err := svc.Home(context.Background(), "client-1", membership.Headers{})
	var denied *membership.AccessError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, membership.CodeMembershipSuspended, denied.Code)
}

func TestHomeShowsASoloDayOnlyWhenOneExists(t *testing.T) {
	store := memory.NewStore()
	require.NoError(t, store.Create(context.Background(), user.User{ID: "maya", Email: "maya@weekset.test", DisplayName: "Maya", AccountType: user.AccountClient}))
	require.NoError(t, store.CreateCenter(context.Background(), port.CenterDraft{CenterID: "center", BranchID: "branch", Timezone: "UTC"}))
	store.PutMemberships("maya", []port.MembershipView{{
		Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch", CenterName: "Harbor",
	}})
	svc := &Service{Centers: store, Users: store, Schedule: store}
	empty, err := svc.Home(context.Background(), "maya", membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	assert.Nil(t, empty.Solo)

	today := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.SaveSolo(context.Background(), port.SoloPlan{
		ID: "plan", FitnessCenterID: "center", BranchID: "branch", ClientUserID: "maya", TrainerUserID: "pt",
		PlanDate: today, Items: []port.SoloItem{{Slug: "squat", Sets: 3, Reps: 8, RestSeconds: 60}},
	}))
	card, err := svc.Home(context.Background(), "maya", membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	require.NotNil(t, card.Solo)
	assert.Equal(t, 1, card.Solo.Count)
	assert.Equal(t, "squat", card.Solo.First)
	rows, err := store.ListBookings(context.Background(), "center", "maya", "", time.Time{}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func newHome() (*Service, *centeruc.Service) {
	store := memory.NewStore()
	hasher := password.Hasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 16}
	authSvc := &authuc.Service{
		Users: store, Tokens: store, Resets: store, Mailer: store, Hasher: hasher,
		Issuer: token.Issuer{Secret: []byte("test-secret"), TTL: 15 * time.Minute},
	}
	return &Service{Centers: store, Users: store}, &centeruc.Service{
		Centers: store, Users: store, Auth: authSvc, Hasher: hasher,
	}
}
