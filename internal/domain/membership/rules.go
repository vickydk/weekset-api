package membership

import "errors"

var (
	// ErrTrainerBranchRequired is a trainer membership with no branch.
	ErrTrainerBranchRequired = errors.New("trainer membership requires a branch")
	// ErrCenterAdminHasBranch is a center-admin membership that names a branch.
	ErrCenterAdminHasBranch = errors.New("center admin has no branch")
	// ErrAlreadyInCenter is a second membership at the same fitness center.
	ErrAlreadyInCenter = errors.New("person is already in the center")
	// ErrTrainerHasBranch is a trainer who already has a membership.
	ErrTrainerHasBranch = errors.New("trainer already has a branch")
	// ErrManagerHasCenter is a manager who already belongs to a fitness center.
	ErrManagerHasCenter = errors.New("manager already belongs to a center")
	// ErrCenterAdminHasCenter is a fitness center account that already has a center.
	ErrCenterAdminHasCenter = errors.New("center admin already has a center")
)

// AcceptMembership reports whether next can be added to the person's existing memberships.
// A client may belong to more than one center. A trainer, a manager, and a center admin may not.
func AcceptMembership(existing []Membership, next Membership) error {
	if next.Role == RoleTrainer && next.BranchID == "" {
		return ErrTrainerBranchRequired
	}
	if next.Role == RoleCenterAdmin && next.BranchID != "" {
		return ErrCenterAdminHasBranch
	}
	for _, have := range existing {
		if have.FitnessCenterID == next.FitnessCenterID {
			return ErrAlreadyInCenter
		}
	}
	if isStaff(next.Role) && len(existing) > 0 {
		return staffCenterError(next.Role)
	}
	if !isStaff(next.Role) && hasStaff(existing) {
		return staffCenterError(staffRole(existing))
	}
	return nil
}

func isStaff(role Role) bool {
	switch role {
	case RoleTrainer, RoleManager, RoleCenterAdmin:
		return true
	default:
		return false
	}
}

func hasStaff(existing []Membership) bool {
	for _, have := range existing {
		if isStaff(have.Role) {
			return true
		}
	}
	return false
}

func staffRole(existing []Membership) Role {
	for _, have := range existing {
		if isStaff(have.Role) {
			return have.Role
		}
	}
	return ""
}

func staffCenterError(role Role) error {
	switch role {
	case RoleTrainer:
		return ErrTrainerHasBranch
	case RoleManager:
		return ErrManagerHasCenter
	default:
		return ErrCenterAdminHasCenter
	}
}
