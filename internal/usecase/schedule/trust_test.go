package scheduleuc

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
	"github.com/vickydk/weekset-api/internal/port"
)

func TestAnotherCenterCannotReadTheLogAndTheAuditKeepsIdsOnly(t *testing.T) {
	store := memory.NewStore()
	require.NoError(t, store.Create(context.Background(), user.User{ID: "pt", Email: "pt@weekset.test", DisplayName: "Alex", AccountType: user.AccountTrainer}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "maya", Email: "maya@weekset.test", DisplayName: "Maya", AccountType: user.AccountClient}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "other", Email: "other@weekset.test", DisplayName: "Sam", AccountType: user.AccountClient}))
	require.NoError(t, store.CreateCenter(context.Background(), port.CenterDraft{CenterID: "center", BranchID: "branch", Timezone: "UTC"}))
	store.PutMemberships("pt", []port.MembershipView{{Role: "trainer", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("maya", []port.MembershipView{{Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("other", []port.MembershipView{{Role: "client", Status: "active", FitnessCenterID: "other-center", BranchID: "other-branch"}})
	require.NoError(t, store.AssignTrainer(context.Background(), port.Assignment{FitnessCenterID: "center", BranchID: "branch", ClientUserID: "maya", TrainerUserID: "pt"}))
	svc := &Service{Schedule: store, Users: store}
	require.NoError(t, svc.ReplaceTemplate(context.Background(), "pt", []WindowInput{{Weekday: int(time.Monday), LocalStart: "07:00", LocalEnd: "08:00"}}, membership.Headers{CenterID: "center"}))
	start := time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC)
	booked, err := svc.Request(context.Background(), "maya", start, start.Add(time.Hour), membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	_, err = svc.WriteLog(context.Background(), "pt", booked.ID, "felt strong", []port.SessionSet{{
		ExerciseName: "squat", Reps: 5, Load: "40", Unit: "kg",
	}}, membership.Headers{CenterID: "center"})
	require.NoError(t, err)

	_, err = svc.ReadLog(context.Background(), "other", booked.ID, membership.Headers{CenterID: "center"})
	var denied *membership.AccessError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, membership.CodeNotFound, denied.Code)

	audits, err := store.ListAudits(context.Background(), "center")
	require.NoError(t, err)
	require.NotEmpty(t, audits)
	raw, err := json.Marshal(audits)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "felt strong")
	assert.NotContains(t, string(raw), "40")
	assert.Equal(t, "session_log", audits[0].EntityType)
	assert.NotEmpty(t, audits[0].EntityID)
}
