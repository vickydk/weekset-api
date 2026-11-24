// Package membership is the access rule for a person at one fitness center.
// Role and status live on the membership. The account type chosen at
// registration does not authorize a route.
package membership

// Role is the membership role. Trainer is the personal trainer.
type Role string

const (
	RoleClient      Role = "client"
	RoleTrainer     Role = "trainer"
	RoleManager     Role = "manager"
	RoleCenterAdmin Role = "center_admin"
)

// Status is the membership status. Removed means the row is gone.
type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

// Valid reports whether the role is one of the four membership roles.
func (r Role) Valid() bool {
	switch r {
	case RoleClient, RoleTrainer, RoleManager, RoleCenterAdmin:
		return true
	default:
		return false
	}
}

// Membership is one person's place in one fitness center.
// A trainer has BranchID. A manager has BranchIDs. A center admin has neither.
// A client has BranchID, the branch of their one trainer at this center.
type Membership struct {
	FitnessCenterID string
	Role            Role
	Status          Status
	BranchID        string
	BranchIDs       []string
}

// Actor is the signed-in person. Access comes from memberships, never from a request body.
type Actor struct {
	UserID      string
	Memberships []Membership
}

// Resource is the center, branch, and people a schedule or log row belongs to.
type Resource struct {
	FitnessCenterID string
	BranchID        string
	ClientUserID    string
	TrainerUserID   string
}

// Scope is the center and branch the caller is allowed to use for this request.
type Scope struct {
	UserID          string
	FitnessCenterID string
	BranchID        string
	Role            Role
}
