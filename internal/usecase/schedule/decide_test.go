package scheduleuc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vickydk/weekset-api/internal/adapter/memory"
	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/schedule"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func TestDeclineReleasesAndConfirmHolds(t *testing.T) {
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
	require.NoError(t, store.AssignTrainer(context.Background(), port.Assignment{FitnessCenterID: "center", BranchID: "branch", ClientUserID: "sam", TrainerUserID: "pt"}))
	svc := &Service{Schedule: store, Users: store}
	require.NoError(t, svc.ReplaceTemplate(context.Background(), "pt", []WindowInput{{Weekday: int(time.Monday), LocalStart: "07:00", LocalEnd: "08:00"}}, membership.Headers{CenterID: "center"}))
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	hours, err := svc.Hours(context.Background(), "maya", from, from.AddDate(0, 0, 1), "", membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	booked, err := svc.Request(context.Background(), "maya", hours.Windows[0].Start, hours.Windows[0].End, membership.Headers{CenterID: "center"})
	require.NoError(t, err)

	confirmed, err := svc.Decide(context.Background(), "pt", booked.ID, string(schedule.StatusConfirmed), membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	assert.Equal(t, "confirmed", confirmed.Status)
	_, err = svc.Request(context.Background(), "sam", hours.Windows[0].Start, hours.Windows[0].End, membership.Headers{CenterID: "center"})
	assert.ErrorIs(t, err, schedule.ErrHourBooked)

	_, err = svc.Decide(context.Background(), "pt", booked.ID, string(schedule.StatusCancelled), membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	again, err := svc.Request(context.Background(), "sam", hours.Windows[0].Start, hours.Windows[0].End, membership.Headers{CenterID: "center"})
	require.NoError(t, err)

	rows, err := svc.Timetable(context.Background(), "manager", from, from.AddDate(0, 0, 1), membership.Headers{BranchID: "branch"})
	require.NoError(t, err)
	var found bool
	for _, row := range rows {
		if row.ID == again.ID {
			found = true
			assert.Equal(t, "Sam", row.ClientName)
			assert.Equal(t, "Alex", row.TrainerName)
		}
		assert.NotContains(t, row.Status, "note")
	}
	assert.True(t, found)
}
