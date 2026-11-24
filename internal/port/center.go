package port

import "context"

// CenterDraft is the one transaction that opens a fitness center.
type CenterDraft struct {
	UserID       string
	Email        string
	PasswordHash string
	DisplayName  string
	CenterID     string
	CenterName   string
	Timezone     string
	BranchID     string
	BranchName   string
	MembershipID string
}

// Branch is one location of a center.
type Branch struct {
	ID              string
	FitnessCenterID string
	Name            string
}

// Invite is a one-time code for a waiting account.
type Invite struct {
	ID              string
	FitnessCenterID string
	Email           string
	Role            string
	BranchIDs       []string
	CodeHash        string
	ExpiresAt       string
}

// CenterStore writes centers, branches, and manager invites.
type CenterStore interface {
	CreateCenter(ctx context.Context, draft CenterDraft) error
	AddBranch(ctx context.Context, centerID, branchID, name string) (Branch, error)
	ListBranches(ctx context.Context, centerID string) ([]Branch, error)
	RenameBranch(ctx context.Context, centerID, branchID, name string) error
	RenameCenter(ctx context.Context, centerID, name, timezone string) error
	SaveInvite(ctx context.Context, invite Invite) error
	ReplaceManagerBranches(ctx context.Context, centerID, userID string, branchIDs []string) error
	BranchBelongs(ctx context.Context, centerID, branchID string) (bool, error)
}
