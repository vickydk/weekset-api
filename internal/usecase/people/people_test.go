package people

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
	centeruc "github.com/vickydk/weekset-api/internal/usecase/center"
)

func TestInviteAcceptAssignAndRemove(t *testing.T) {
	svc, centers, store, now := newPeople()
	admin := openHarbor(t, centers)
	centerID := admin.Profile.Memberships[0].FitnessCenterID
	branchID := admin.Profile.Memberships[0].Branches[0].ID
	putManager(store, "manager-1", centerID, branchID)
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "trainer-1", Email: "alex@harbor.club", PasswordHash: "x", DisplayName: "Alex", AccountType: user.AccountTrainer,
	}))
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "client-1", Email: "maya@harbor.club", PasswordHash: "x", DisplayName: "Maya", AccountType: user.AccountClient,
	}))

	_, trainerCode, _, err := svc.Invite(context.Background(), "manager-1", "alex@harbor.club", "trainer", branchID, membership.Headers{BranchID: branchID})
	require.NoError(t, err)
	_, clientCode, _, err := svc.Invite(context.Background(), "manager-1", "maya@harbor.club", "client", branchID, membership.Headers{BranchID: branchID})
	require.NoError(t, err)

	page, err := svc.People(context.Background(), "manager-1", membership.Headers{BranchID: branchID})
	require.NoError(t, err)
	require.Len(t, page.Invites, 2)

	require.NoError(t, svc.Accept(context.Background(), trainerCode))
	require.NoError(t, svc.Accept(context.Background(), clientCode))
	assert.ErrorIs(t, svc.Accept(context.Background(), trainerCode), ErrInviteInvalid)
	assert.ErrorIs(t, svc.Accept(context.Background(), "not-a-code"), ErrInviteInvalid)

	trainerRows, err := store.Memberships(context.Background(), "trainer-1")
	require.NoError(t, err)
	require.Len(t, trainerRows, 1)
	assert.Equal(t, "trainer", trainerRows[0].Role)
	assert.Equal(t, branchID, trainerRows[0].BranchID)

	require.NoError(t, svc.Assign(context.Background(), "manager-1", "client-1", "trainer-1", membership.Headers{BranchID: branchID}))
	page, err = svc.People(context.Background(), "manager-1", membership.Headers{BranchID: branchID})
	require.NoError(t, err)
	require.Len(t, page.People, 2)
	var maya port.Person
	for _, person := range page.People {
		if person.UserID == "client-1" {
			maya = person
		}
	}
	assert.Equal(t, "Alex", maya.TrainerName)
	assert.Empty(t, page.Invites)

	require.NoError(t, svc.Suspend(context.Background(), "manager-1", "client-1", membership.Headers{BranchID: branchID}))
	clientRows, err := store.Memberships(context.Background(), "client-1")
	require.NoError(t, err)
	assert.Equal(t, "suspended", clientRows[0].Status)
	require.NoError(t, svc.Restore(context.Background(), "manager-1", "client-1", membership.Headers{BranchID: branchID}))

	require.NoError(t, svc.Remove(context.Background(), "manager-1", "client-1", membership.Headers{BranchID: branchID}))
	clientRows, err = store.Memberships(context.Background(), "client-1")
	require.NoError(t, err)
	assert.Empty(t, clientRows)
	_, err = store.FindByID(context.Background(), "client-1")
	assert.NoError(t, err)

	*now = now.Add(8 * 24 * time.Hour)
	require.NoError(t, store.SaveInvite(context.Background(), port.Invite{
		ID: "old", FitnessCenterID: centerID, Email: "maya@harbor.club", Role: "client",
		BranchIDs: []string{branchID}, CodeHash: hashCode("expired-code"),
		ExpiresAt: time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC).Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}))
	assert.ErrorIs(t, svc.Accept(context.Background(), "expired-code"), ErrInviteInvalid)
}

func TestManagerCannotInviteOrSuspendAManager(t *testing.T) {
	svc, centers, store, _ := newPeople()
	admin := openHarbor(t, centers)
	centerID := admin.Profile.Memberships[0].FitnessCenterID
	branchID := admin.Profile.Memberships[0].Branches[0].ID
	putManager(store, "manager-1", centerID, branchID)
	putManager(store, "manager-2", centerID, branchID)

	_, _, _, err := svc.Invite(context.Background(), "manager-1", "lee2@harbor.club", "manager", branchID, membership.Headers{BranchID: branchID})
	var denied *membership.AccessError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, membership.CodeForbidden, denied.Code)

	err = svc.Suspend(context.Background(), "manager-1", "manager-2", membership.Headers{BranchID: branchID})
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, membership.CodeForbidden, denied.Code)
}

func TestATrainerCannotTakeASecondBranch(t *testing.T) {
	svc, centers, store, _ := newPeople()
	admin := openHarbor(t, centers)
	centerID := admin.Profile.Memberships[0].FitnessCenterID
	branchID := admin.Profile.Memberships[0].Branches[0].ID
	other, err := centers.AddBranch(context.Background(), admin.Profile.ID, "Kemang")
	require.NoError(t, err)
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "trainer-1", Email: "alex@harbor.club", PasswordHash: "x", DisplayName: "Alex", AccountType: user.AccountTrainer,
	}))
	_, code, _, err := svc.Invite(context.Background(), admin.Profile.ID, "alex@harbor.club", "trainer", branchID, membership.Headers{})
	require.NoError(t, err)
	require.NoError(t, svc.Accept(context.Background(), code))

	_, _, _, err = svc.Invite(context.Background(), admin.Profile.ID, "alex@harbor.club", "trainer", other.ID, membership.Headers{})
	assert.ErrorIs(t, err, membership.ErrTrainerHasBranch)

	require.NoError(t, store.SaveInvite(context.Background(), port.Invite{
		ID: "second", FitnessCenterID: centerID, Email: "alex@harbor.club", Role: "trainer",
		BranchIDs: []string{other.ID}, CodeHash: hashCode("second-code"),
		ExpiresAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}))
	assert.ErrorIs(t, svc.Accept(context.Background(), "second-code"), membership.ErrTrainerHasBranch)
}

func TestAClientAlreadyInTheCenterIsRejected(t *testing.T) {
	svc, centers, store, _ := newPeople()
	admin := openHarbor(t, centers)
	branchID := admin.Profile.Memberships[0].Branches[0].ID
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "client-1", Email: "maya@harbor.club", PasswordHash: "x", DisplayName: "Maya", AccountType: user.AccountClient,
	}))
	_, code, _, err := svc.Invite(context.Background(), admin.Profile.ID, "maya@harbor.club", "client", branchID, membership.Headers{})
	require.NoError(t, err)
	require.NoError(t, svc.Accept(context.Background(), code))
	_, _, _, err = svc.Invite(context.Background(), admin.Profile.ID, "maya@harbor.club", "client", branchID, membership.Headers{})
	assert.ErrorIs(t, err, membership.ErrAlreadyInCenter)
}

func TestSuspendLeavesAnotherCenterActive(t *testing.T) {
	svc, centers, store, _ := newPeople()
	admin := openHarbor(t, centers)
	centerID := admin.Profile.Memberships[0].FitnessCenterID
	branchID := admin.Profile.Memberships[0].Branches[0].ID
	require.NoError(t, store.Create(context.Background(), user.User{
		ID: "client-1", Email: "maya@harbor.club", PasswordHash: "x", DisplayName: "Maya", AccountType: user.AccountClient,
	}))
	_, code, _, err := svc.Invite(context.Background(), admin.Profile.ID, "maya@harbor.club", "client", branchID, membership.Headers{})
	require.NoError(t, err)
	require.NoError(t, svc.Accept(context.Background(), code))
	rows, err := store.Memberships(context.Background(), "client-1")
	require.NoError(t, err)
	rows = append(rows, port.MembershipView{
		Role: "client", Status: "active", FitnessCenterID: "other-center", CenterName: "North", BranchID: "other-branch",
	})
	store.PutMemberships("client-1", rows)

	require.NoError(t, svc.Suspend(context.Background(), admin.Profile.ID, "client-1", membership.Headers{}))
	rows, err = store.Memberships(context.Background(), "client-1")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		if row.FitnessCenterID == centerID {
			assert.Equal(t, "suspended", row.Status)
		} else {
			assert.Equal(t, "active", row.Status)
		}
	}
}

func TestCenterListIncludesManagers(t *testing.T) {
	svc, centers, store, _ := newPeople()
	admin := openHarbor(t, centers)
	centerID := admin.Profile.Memberships[0].FitnessCenterID
	branchID := admin.Profile.Memberships[0].Branches[0].ID
	putManager(store, "manager-1", centerID, branchID)
	page, err := svc.People(context.Background(), admin.Profile.ID, membership.Headers{})
	require.NoError(t, err)
	require.Len(t, page.People, 1)
	assert.Equal(t, "manager", page.People[0].Role)
	assert.Equal(t, "Downtown", page.People[0].BranchName)

	managerPage, err := svc.People(context.Background(), "manager-1", membership.Headers{BranchID: branchID})
	require.NoError(t, err)
	assert.Empty(t, managerPage.People)
}

func newPeople() (*Service, *centeruc.Service, *memory.Store, *time.Time) {
	store := memory.NewStore()
	now := time.Date(2026, 11, 24, 6, 0, 0, 0, time.UTC)
	clock := now
	hasher := password.Hasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 16}
	authSvc := &authuc.Service{
		Users: store, Tokens: store, Resets: store, Mailer: store, Hasher: hasher,
		Issuer: token.Issuer{Secret: []byte("test-secret"), TTL: 15 * time.Minute},
		Now:    func() time.Time { return clock },
	}
	centers := &centeruc.Service{Centers: store, Users: store, Auth: authSvc, Hasher: hasher, Now: func() time.Time { return clock }}
	svc := &Service{Centers: store, Users: store, Now: func() time.Time { return clock }}
	return svc, centers, store, &clock
}

func openHarbor(t *testing.T, centers *centeruc.Service) authuc.Session {
	t.Helper()
	session, err := centers.Open(context.Background(), centeruc.OpenInput{
		Email: "ada@harbor.club", Password: "long-enough", DisplayName: "Ada",
		CenterName: "Harbor Club", BranchName: "Downtown", Timezone: "Asia/Jakarta",
	})
	require.NoError(t, err)
	return session
}

func putManager(store *memory.Store, userID, centerID, branchID string) {
	_ = store.Create(context.Background(), user.User{
		ID: userID, Email: userID + "@harbor.club", PasswordHash: "x", DisplayName: "Lee", AccountType: user.AccountManager,
	})
	store.PutMemberships(userID, []port.MembershipView{{
		Role: "manager", Status: "active", FitnessCenterID: centerID, CenterName: "Harbor Club",
		Branches: []port.BranchView{{ID: branchID, Name: "Downtown"}},
	}})
}
