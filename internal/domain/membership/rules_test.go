package membership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCanJoinTwoCenters(t *testing.T) {
	existing := []Membership{{
		FitnessCenterID: "center-a",
		Role:            RoleClient,
		Status:          StatusActive,
		BranchID:        "downtown",
	}}
	err := AcceptMembership(existing, Membership{
		FitnessCenterID: "center-b",
		Role:            RoleClient,
		Status:          StatusActive,
		BranchID:        "north",
	})
	require.NoError(t, err)
}

func TestClientCannotJoinTheSameCenterTwice(t *testing.T) {
	existing := []Membership{{
		FitnessCenterID: "center-a",
		Role:            RoleClient,
		Status:          StatusActive,
	}}
	err := AcceptMembership(existing, Membership{
		FitnessCenterID: "center-a",
		Role:            RoleClient,
		Status:          StatusActive,
	})
	assert.ErrorIs(t, err, ErrAlreadyInCenter)
}

func TestTrainerMembershipRequiresABranch(t *testing.T) {
	err := AcceptMembership(nil, Membership{
		FitnessCenterID: "center-a",
		Role:            RoleTrainer,
		Status:          StatusActive,
	})
	assert.ErrorIs(t, err, ErrTrainerBranchRequired)
}

func TestTrainerCannotJoinASecondCenter(t *testing.T) {
	existing := []Membership{{
		FitnessCenterID: "center-a",
		Role:            RoleTrainer,
		Status:          StatusActive,
		BranchID:        "downtown",
	}}
	err := AcceptMembership(existing, Membership{
		FitnessCenterID: "center-b",
		Role:            RoleTrainer,
		Status:          StatusActive,
		BranchID:        "north",
	})
	assert.ErrorIs(t, err, ErrTrainerHasBranch)
}

func TestManagerCannotJoinASecondCenter(t *testing.T) {
	existing := []Membership{{
		FitnessCenterID: "center-a",
		Role:            RoleManager,
		Status:          StatusActive,
		BranchIDs:       []string{"downtown"},
	}}
	err := AcceptMembership(existing, Membership{
		FitnessCenterID: "center-b",
		Role:            RoleManager,
		Status:          StatusActive,
		BranchIDs:       []string{"north"},
	})
	assert.ErrorIs(t, err, ErrManagerHasCenter)
}

func TestStaffMembershipBlocksAClientCenter(t *testing.T) {
	existing := []Membership{{
		FitnessCenterID: "center-a",
		Role:            RoleTrainer,
		Status:          StatusActive,
		BranchID:        "downtown",
	}}
	err := AcceptMembership(existing, Membership{
		FitnessCenterID: "center-b",
		Role:            RoleClient,
		Status:          StatusActive,
		BranchID:        "north",
	})
	assert.ErrorIs(t, err, ErrTrainerHasBranch)
}

func TestCenterAdminHasNoBranch(t *testing.T) {
	err := AcceptMembership(nil, Membership{
		FitnessCenterID: "center-a",
		Role:            RoleCenterAdmin,
		Status:          StatusActive,
		BranchID:        "downtown",
	})
	assert.ErrorIs(t, err, ErrCenterAdminHasBranch)
}
