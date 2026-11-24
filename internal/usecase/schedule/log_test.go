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

func TestWritingALogCompletesTheBooking(t *testing.T) {
	store := memory.NewStore()
	require.NoError(t, store.Create(context.Background(), user.User{ID: "pt", Email: "pt@weekset.test", DisplayName: "Alex", AccountType: user.AccountTrainer}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "maya", Email: "maya@weekset.test", DisplayName: "Maya", AccountType: user.AccountClient}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "sam", Email: "sam@weekset.test", DisplayName: "Sam", AccountType: user.AccountClient}))
	require.NoError(t, store.CreateCenter(context.Background(), port.CenterDraft{CenterID: "center", BranchID: "branch", Timezone: "UTC"}))
	store.PutMemberships("pt", []port.MembershipView{{Role: "trainer", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("maya", []port.MembershipView{{Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("sam", []port.MembershipView{{Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("manager", []port.MembershipView{{
		Role: "manager", Status: "active", FitnessCenterID: "center", BranchID: "branch",
		Branches: []port.BranchView{{ID: "branch", Name: "Downtown"}},
	}})
	require.NoError(t, store.AssignTrainer(context.Background(), port.Assignment{FitnessCenterID: "center", BranchID: "branch", ClientUserID: "maya", TrainerUserID: "pt"}))
	svc := &Service{Schedule: store, Users: store}
	require.NoError(t, svc.ReplaceTemplate(context.Background(), "pt", []WindowInput{{Weekday: int(time.Monday), LocalStart: "07:00", LocalEnd: "08:00"}}, membership.Headers{CenterID: "center"}))
	start := time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC)
	booked, err := svc.Request(context.Background(), "maya", start, start.Add(time.Hour), membership.Headers{CenterID: "center"})
	require.NoError(t, err)

	log, err := svc.WriteLog(context.Background(), "pt", booked.ID, "felt strong", []port.SessionSet{{
		ExerciseName: "squat", Reps: 5, Load: "40", Unit: "kg",
	}}, membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	assert.Equal(t, "felt strong", log.Note)
	require.Len(t, log.Sets, 1)

	saved, err := store.FindBooking(context.Background(), "center", booked.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", saved.Status)

	_, err = svc.ReadLog(context.Background(), "sam", booked.ID, membership.Headers{CenterID: "center"})
	assert.ErrorIs(t, err, user.ErrNotFound)
	_, err = svc.ReadLog(context.Background(), "manager", booked.ID, membership.Headers{BranchID: "branch"})
	assert.ErrorIs(t, err, user.ErrNotFound)
	own, err := svc.ReadLog(context.Background(), "maya", booked.ID, membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	assert.Equal(t, "40", own.Sets[0].Load)
}
