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

func TestTemplateCloseAndClientRead(t *testing.T) {
	store := memory.NewStore()
	require.NoError(t, store.Create(context.Background(), user.User{ID: "pt", Email: "pt@weekset.test", DisplayName: "Alex", AccountType: user.AccountTrainer}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "maya", Email: "maya@weekset.test", DisplayName: "Maya", AccountType: user.AccountClient}))
	require.NoError(t, store.Create(context.Background(), user.User{ID: "other", Email: "other@weekset.test", DisplayName: "Sam", AccountType: user.AccountClient}))
	require.NoError(t, store.CreateCenter(context.Background(), port.CenterDraft{
		CenterID: "center", BranchID: "branch", Timezone: "Asia/Jakarta",
	}))
	store.PutMemberships("pt", []port.MembershipView{{
		Role: "trainer", Status: "active", FitnessCenterID: "center", BranchID: "branch",
	}})
	store.PutMemberships("maya", []port.MembershipView{{
		Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch",
	}})
	store.PutMemberships("other", []port.MembershipView{{
		Role: "client", Status: "active", FitnessCenterID: "center", BranchID: "branch",
	}})
	require.NoError(t, store.AssignTrainer(context.Background(), port.Assignment{
		FitnessCenterID: "center", BranchID: "branch", ClientUserID: "maya", TrainerUserID: "pt",
	}))
	svc := &Service{Schedule: store, Users: store}
	err := svc.ReplaceTemplate(context.Background(), "pt", []WindowInput{
		{Weekday: int(time.Monday), LocalStart: "07:00", LocalEnd: "08:00"},
	}, membership.Headers{CenterID: "center"})
	require.NoError(t, err)

	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 7)
	hours, err := svc.Hours(context.Background(), "maya", from, to, "", membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	require.Len(t, hours.Windows, 1)
	assert.False(t, hours.Trainer)
	assert.Empty(t, hours.Closed)

	_, err = svc.Hours(context.Background(), "other", from, to, "pt", membership.Headers{CenterID: "center"})
	assert.ErrorIs(t, err, user.ErrNotFound)

	id, err := svc.CloseHour(context.Background(), "pt", hours.Windows[0].Start, hours.Windows[0].End, membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	own, err := svc.Hours(context.Background(), "pt", from, to, "", membership.Headers{CenterID: "center"})
	require.NoError(t, err)
	assert.Empty(t, own.Windows)
	require.Len(t, own.Closed, 1)
	require.Len(t, own.Template, 1)

	require.NoError(t, svc.ReopenHour(context.Background(), "pt", id, membership.Headers{CenterID: "center"}))
	err = svc.ReplaceTemplate(context.Background(), "pt", []WindowInput{
		{Weekday: 1, LocalStart: "07:00", LocalEnd: "09:00"},
		{Weekday: 1, LocalStart: "08:00", LocalEnd: "10:00"},
	}, membership.Headers{CenterID: "center"})
	var access *membership.AccessError
	require.ErrorAs(t, err, &access)
	assert.Equal(t, membership.CodeValidation, access.Code)
}
