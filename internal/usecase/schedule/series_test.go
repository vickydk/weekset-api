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

func TestRepeatMakesEightWeeksAndSkipChangesOneDate(t *testing.T) {
	store := memory.NewStore()
	require.NoError(t, store.Create(context.Background(), user.User{ID: "pt", Email: "pt@weekset.test", DisplayName: "Alex", AccountType: user.AccountTrainer}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "maya", Email: "maya@weekset.test", DisplayName: "Maya", AccountType: user.AccountClient}))
	require.NoError(t, store.CreateCenter(context.Background(), port.CenterDraft{CenterID: "center", BranchID: "branch", Timezone: "UTC"}))
	store.PutMemberships("pt", []port.MembershipView{{Role: "trainer", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	store.PutMemberships("maya", []port.MembershipView{{Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch"}})
	require.NoError(t, store.AssignTrainer(context.Background(), port.Assignment{FitnessCenterID: "center", BranchID: "branch", ClientUserID: "maya", TrainerUserID: "pt"}))
	svc := &Service{Schedule: store, Users: store}
	require.NoError(t, svc.ReplaceTemplate(context.Background(), "pt", []WindowInput{{Weekday: int(time.Monday), LocalStart: "07:00", LocalEnd: "08:00"}}, membership.Headers{CenterID: "center"}))
	start := time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	seriesID, err := svc.Repeat(context.Background(), "maya", start, end, membership.Headers{CenterID: "center"})
	require.NoError(t, err)

	rows, err := store.SeriesBookings(context.Background(), "center", seriesID)
	require.NoError(t, err)
	require.Len(t, rows, schedule.SeriesWeeks)
	require.NoError(t, svc.Skip(context.Background(), "maya", seriesID, "2026-11-23", membership.Headers{CenterID: "center"}))
	rows, err = store.SeriesBookings(context.Background(), "center", seriesID)
	require.NoError(t, err)
	cancelled := 0
	requested := 0
	for _, row := range rows {
		if row.StartsAt.Equal(start.AddDate(0, 0, 7)) {
			assert.Equal(t, "requested", row.Status)
		}
		switch row.Status {
		case "cancelled":
			cancelled++
		case "requested":
			requested++
		}
	}
	assert.Equal(t, 1, cancelled)
	assert.Equal(t, 7, requested)
}
