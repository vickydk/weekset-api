package membership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	centerA = "center-a"
	centerB = "center-b"
	down    = "downtown"
	north   = "north"
	client  = "user-client"
	trainer = "user-trainer"
	otherPT = "user-other-pt"
)

func clientActor() Actor {
	return Actor{UserID: client, Memberships: []Membership{{
		FitnessCenterID: centerA,
		Role:            RoleClient,
		Status:          StatusActive,
		BranchID:        down,
	}}}
}

func trainerActor() Actor {
	return Actor{UserID: trainer, Memberships: []Membership{{
		FitnessCenterID: centerA,
		Role:            RoleTrainer,
		Status:          StatusActive,
		BranchID:        down,
	}}}
}

func managerActor() Actor {
	return Actor{UserID: "user-manager", Memberships: []Membership{{
		FitnessCenterID: centerA,
		Role:            RoleManager,
		Status:          StatusActive,
		BranchIDs:       []string{down, north},
	}}}
}

func centerActor() Actor {
	return Actor{UserID: "user-center", Memberships: []Membership{{
		FitnessCenterID: centerA,
		Role:            RoleCenterAdmin,
		Status:          StatusActive,
	}}}
}

func ownLog() Resource {
	return Resource{
		FitnessCenterID: centerA,
		BranchID:        down,
		ClientUserID:    client,
		TrainerUserID:   trainer,
	}
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	var access *AccessError
	require.ErrorAs(t, err, &access)
	return access.Code
}

func TestRoleValid(t *testing.T) {
	assert.True(t, RoleClient.Valid())
	assert.True(t, RoleTrainer.Valid())
	assert.True(t, RoleManager.Valid())
	assert.True(t, RoleCenterAdmin.Valid())
	assert.False(t, Role("owner").Valid())
}

func TestAccessMatrix(t *testing.T) {
	assigned := ownLog()
	unassigned := assigned
	unassigned.TrainerUserID = otherPT
	otherClient := assigned
	otherClient.ClientUserID = "someone-else"

	tests := []struct {
		name string
		err  error
		code string
	}{
		{"client reads own schedule", CanReadSchedule(clientActor(), centerA, down), ""},
		{"client reads other center", CanReadSchedule(clientActor(), centerB, down), CodeNotFound},
		{"client reads other branch", CanReadSchedule(clientActor(), centerA, north), CodeNotFound},
		{"client reads own log", CanReadSessionLog(clientActor(), assigned), ""},
		{"client reads another client", CanReadSessionLog(clientActor(), otherClient), CodeNotFound},
		{"client writes log", CanWriteSessionLog(clientActor(), assigned), CodeForbidden},
		{"client manages people", CanManagePeople(clientActor(), centerA, down, RoleTrainer), CodeForbidden},
		{"client manages branches", CanManageBranches(clientActor(), centerA), CodeForbidden},

		{"trainer reads own branch", CanReadSchedule(trainerActor(), centerA, down), ""},
		{"trainer reads other branch", CanReadSchedule(trainerActor(), centerA, north), CodeNotFound},
		{"trainer reads other center", CanReadSchedule(trainerActor(), centerB, down), CodeNotFound},
		{"trainer reads assigned log", CanReadSessionLog(trainerActor(), assigned), ""},
		{"trainer reads unassigned client", CanReadSessionLog(trainerActor(), unassigned), CodeNotFound},
		{"trainer writes assigned log", CanWriteSessionLog(trainerActor(), assigned), ""},
		{"trainer writes unassigned log", CanWriteSessionLog(trainerActor(), unassigned), CodeNotFound},
		{"trainer manages people", CanManagePeople(trainerActor(), centerA, down, RoleClient), CodeForbidden},
		{"trainer manages branches", CanManageBranches(trainerActor(), centerA), CodeForbidden},

		{"manager reads own branch", CanReadSchedule(managerActor(), centerA, down), ""},
		{"manager reads other branch", CanReadSchedule(managerActor(), centerA, "riverside"), CodeNotFound},
		{"manager reads a note", CanReadSessionLog(managerActor(), assigned), CodeForbidden},
		{"manager writes a note", CanWriteSessionLog(managerActor(), assigned), CodeForbidden},
		{"manager manages client", CanManagePeople(managerActor(), centerA, down, RoleClient), ""},
		{"manager manages trainer", CanManagePeople(managerActor(), centerA, north, RoleTrainer), ""},
		{"manager manages another manager", CanManagePeople(managerActor(), centerA, down, RoleManager), CodeForbidden},
		{"manager manages other branch", CanManagePeople(managerActor(), centerA, "riverside", RoleClient), CodeNotFound},
		{"manager creates a branch", CanManageBranches(managerActor(), centerA), CodeForbidden},

		{"center reads any branch", CanReadSchedule(centerActor(), centerA, north), ""},
		{"center reads other center", CanReadSchedule(centerActor(), centerB, down), CodeNotFound},
		{"center reads a note", CanReadSessionLog(centerActor(), assigned), CodeForbidden},
		{"center writes a note", CanWriteSessionLog(centerActor(), assigned), CodeForbidden},
		{"center manages manager", CanManagePeople(centerActor(), centerA, down, RoleManager), ""},
		{"center manages center admin", CanManagePeople(centerActor(), centerA, down, RoleCenterAdmin), CodeForbidden},
		{"center creates a branch", CanManageBranches(centerActor(), centerA), ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.code == "" {
				assert.NoError(t, test.err)
				return
			}
			assert.Equal(t, test.code, codeOf(t, test.err))
		})
	}
}

func TestSuspendedMembership(t *testing.T) {
	actor := clientActor()
	actor.Memberships[0].Status = StatusSuspended

	err := CanReadSchedule(actor, centerA, down)
	access := err.(*AccessError)
	assert.Equal(t, CodeMembershipSuspended, access.Code)
	assert.Equal(t, 403, access.StatusCode())

	other := Actor{UserID: client, Memberships: []Membership{
		actor.Memberships[0],
		{FitnessCenterID: centerB, Role: RoleClient, Status: StatusActive, BranchID: north},
	}}
	assert.NoError(t, CanReadSchedule(other, centerB, north))
	assert.Equal(t, CodeMembershipSuspended, codeOf(t, CanReadSchedule(other, centerA, down)))
}

func TestOutsideCallerIsNotFound(t *testing.T) {
	err := CanReadSchedule(trainerActor(), centerA, north).(*AccessError)
	assert.Equal(t, CodeNotFound, err.Code)
	assert.Equal(t, 404, err.StatusCode())
}

func TestResolveIgnoresBodyAndTrainerBranchHeader(t *testing.T) {
	scope, err := Resolve(clientActor(), Headers{}, BodyClaim{})
	require.NoError(t, err)
	assert.Equal(t, centerA, scope.FitnessCenterID)
	assert.Equal(t, down, scope.BranchID)

	_, err = Resolve(clientActor(), Headers{CenterID: centerA}, BodyClaim{FitnessCenterID: centerB})
	assert.Equal(t, CodeForbidden, codeOf(t, err))

	_, err = Resolve(trainerActor(), Headers{}, BodyClaim{Role: string(RoleCenterAdmin)})
	assert.Equal(t, CodeForbidden, codeOf(t, err))

	_, err = Resolve(managerActor(), Headers{BranchID: down}, BodyClaim{BranchID: north})
	assert.Equal(t, CodeForbidden, codeOf(t, err))

	scope, err = Resolve(trainerActor(), Headers{CenterID: centerA, BranchID: north}, BodyClaim{})
	require.NoError(t, err)
	assert.Equal(t, down, scope.BranchID)

	two := clientActor()
	two.Memberships = append(two.Memberships, Membership{
		FitnessCenterID: centerB,
		Role:            RoleClient,
		Status:          StatusActive,
		BranchID:        north,
	})
	_, err = Resolve(two, Headers{}, BodyClaim{})
	access := err.(*AccessError)
	assert.Equal(t, CodeValidation, access.Code)
	assert.Equal(t, "choose a center", access.Message)
	assert.Equal(t, 400, access.StatusCode())

	scope, err = Resolve(two, Headers{CenterID: centerB}, BodyClaim{})
	require.NoError(t, err)
	assert.Equal(t, centerB, scope.FitnessCenterID)

	_, err = Resolve(managerActor(), Headers{CenterID: centerA}, BodyClaim{})
	assert.Equal(t, "choose a branch", err.Error())
}

func TestManagerWithOneBranchNeedsNoHeader(t *testing.T) {
	actor := managerActor()
	actor.Memberships[0].BranchIDs = []string{down}

	scope, err := Resolve(actor, Headers{}, BodyClaim{})
	require.NoError(t, err)
	assert.Equal(t, down, scope.BranchID)
}
