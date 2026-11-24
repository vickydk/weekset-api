// Package port holds the outbound interfaces the use cases call.
package port

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/user"
)

// MembershipView is one row of GET /v1/me.
type MembershipView struct {
	Role            string
	Status          string
	FitnessCenterID string
	CenterName      string
	BranchID        string
	BranchName      string
	Branches        []BranchView
	TrainerUserID   string
}

// BranchView is one branch a manager covers, or the center's branch list.
type BranchView struct {
	ID   string
	Name string
}

// UserRepository stores accounts.
type UserRepository interface {
	Create(ctx context.Context, account user.User) error
	FindByEmail(ctx context.Context, email string) (user.User, error)
	FindByID(ctx context.Context, id string) (user.User, error)
	UpdatePassword(ctx context.Context, id, passwordHash string) error
	Memberships(ctx context.Context, userID string) ([]MembershipView, error)
}

// RefreshToken is one stored opaque token.
type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// RefreshStore keeps hashed refresh tokens.
type RefreshStore interface {
	SaveRefresh(ctx context.Context, token RefreshToken) error
	FindRefresh(ctx context.Context, tokenHash string) (RefreshToken, error)
	RevokeRefresh(ctx context.Context, id string, at time.Time) error
	RevokeFamily(ctx context.Context, familyID string, at time.Time) error
	RevokeUserRefresh(ctx context.Context, userID string, at time.Time) error
}

// PasswordReset is a single-use reset.
type PasswordReset struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// ResetStore keeps hashed reset tokens.
type ResetStore interface {
	SaveReset(ctx context.Context, reset PasswordReset) error
	FindReset(ctx context.Context, tokenHash string) (PasswordReset, error)
	MarkResetUsed(ctx context.Context, id string, at time.Time) error
}

// Mailer delivers a reset token. It must not log the token.
type Mailer interface {
	SendPasswordReset(ctx context.Context, email, token string) error
}
