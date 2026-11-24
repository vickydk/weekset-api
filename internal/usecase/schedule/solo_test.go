package scheduleuc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vickydk/weekset-api/internal/adapter/memory"
	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func TestSoloDayDoesNotBookAnHour(t *testing.T) {
	store := memory.NewStore()
	require.NoError(t, store.Create(context.Background(), user.User{ID: "pt", Email: "pt@weekset.test", DisplayName: "Alex", AccountType: user.AccountTrainer}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "maya", Email: "maya@weekset.test", DisplayName: "Maya", AccountType: user.AccountClient}))
	require.NoError(t, store.CreateCenter(context.Background(), port.CenterDraft{CenterID: "center", BranchID: "branch", Timezone: "UTC"}))
	store.PutMemberships("pt", []port.MembershipView{{Role: "trainer", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("maya", []port.MembershipView{{Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	require.NoError(t, store.AssignTrainer(context.Background(), port.Assignment{FitnessCenterID: "center", BranchID: "branch", ClientUserID: "maya", TrainerUserID: "pt"}))
	svc := &Service{Schedule: store, Users: store}

	plan, err := svc.AssignSolo(context.Background(), "pt", "maya", "2026-11-24", []port.SoloItem{{
		Slug: "squat", Sets: 3, Reps: 8, RestSeconds: 60,
	}}, membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	assert.Equal(t, "2026-11-24", plan.PlanDate)

	rows, err := store.ListBookings(context.Background(), "center", "maya", "pt", time.Time{}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, rows)
	got, err := svc.Solo(context.Background(), "maya", "", "2026-11-24", membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "squat", got.Items[0].Slug)
}
