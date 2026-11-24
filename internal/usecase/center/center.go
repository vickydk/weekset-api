package center

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/password"
	"github.com/vickydk/weekset-api/internal/port"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
)

// Service opens a fitness center and keeps its branches.
type Service struct {
	Centers port.CenterStore
	Users   port.UserRepository
	Auth    *authuc.Service
	Hasher  password.Hasher
	Now     func() time.Time
}

// OpenInput is the public center registration.
type OpenInput struct {
	Email       string
	Password    string
	DisplayName string
	CenterName  string
	BranchName  string
	Timezone    string
}

// Open creates the user, the center, the first branch, and the super-admin membership.
func (s *Service) Open(ctx context.Context, in OpenInput) (authuc.Session, error) {
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return authuc.Session{}, fmt.Errorf("timezone")
	}
	hash, err := s.Hasher.Hash(in.Password)
	if err != nil {
		return authuc.Session{}, err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	err = s.Centers.CreateCenter(ctx, port.CenterDraft{
		UserID:       newID(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		CenterID:     newID(),
		CenterName:   strings.TrimSpace(in.CenterName),
		Timezone:     in.Timezone,
		BranchID:     newID(),
		BranchName:   strings.TrimSpace(in.BranchName),
		MembershipID: newID(),
	})
	if err != nil {
		return authuc.Session{}, err
	}
	return s.Auth.StartSession(ctx, email)
}

// AddBranch creates a branch for the caller's center.
func (s *Service) AddBranch(ctx context.Context, userID, name string) (port.Branch, error) {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return port.Branch{}, err
	}
	centerID := actor.Memberships[0].FitnessCenterID
	if err := membership.CanManageBranches(actor, centerID); err != nil {
		return port.Branch{}, err
	}
	return s.Centers.AddBranch(ctx, centerID, newID(), strings.TrimSpace(name))
}

// Branches lists what this caller may see.
func (s *Service) Branches(ctx context.Context, userID string) ([]port.Branch, error) {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(actor.Memberships) == 0 {
		return nil, user.ErrUnauthorized
	}
	row := actor.Memberships[0]
	all, err := s.Centers.ListBranches(ctx, row.FitnessCenterID)
	if err != nil {
		return nil, err
	}
	switch row.Role {
	case membership.RoleCenterAdmin:
		return all, nil
	case membership.RoleManager:
		allowed := map[string]bool{}
		for _, id := range row.BranchIDs {
			allowed[id] = true
		}
		return filterBranches(all, allowed), nil
	default:
		allowed := map[string]bool{row.BranchID: true}
		return filterBranches(all, allowed), nil
	}
}

// UpdateCenter changes the name or the timezone. Empty fields stay as they are.
func (s *Service) UpdateCenter(ctx context.Context, userID, name, timezone string) error {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return err
	}
	centerID := actor.Memberships[0].FitnessCenterID
	if err := membership.CanManageBranches(actor, centerID); err != nil {
		return err
	}
	if timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return fmt.Errorf("timezone")
		}
	}
	return s.Centers.RenameCenter(ctx, centerID, strings.TrimSpace(name), timezone)
}

// RenameBranch changes one branch of this center.
func (s *Service) RenameBranch(ctx context.Context, userID, branchID, name string) error {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return err
	}
	centerID := actor.Memberships[0].FitnessCenterID
	if err := membership.CanManageBranches(actor, centerID); err != nil {
		return err
	}
	ok, err := s.Centers.BranchBelongs(ctx, centerID, branchID)
	if err != nil {
		return err
	}
	if !ok {
		return user.ErrNotFound
	}
	return s.Centers.RenameBranch(ctx, centerID, branchID, strings.TrimSpace(name))
}

// InviteManager creates a one-time code for a registered manager with no center.
func (s *Service) InviteManager(ctx context.Context, userID, email string, branchIDs []string) (id, code string, expires time.Time, err error) {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	centerID := actor.Memberships[0].FitnessCenterID
	if err := membership.CanManagePeople(actor, centerID, "", membership.RoleManager); err != nil {
		return "", "", time.Time{}, err
	}
	account, findErr := s.Users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if findErr != nil || account.AccountType != user.AccountManager {
		return "", "", time.Time{}, fmt.Errorf("account is not waiting as a manager")
	}
	rows, err := s.Users.Memberships(ctx, account.ID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	if len(rows) > 0 {
		return "", "", time.Time{}, fmt.Errorf("manager already belongs to a center")
	}
	for _, branchID := range branchIDs {
		ok, err := s.Centers.BranchBelongs(ctx, centerID, branchID)
		if err != nil {
			return "", "", time.Time{}, err
		}
		if !ok {
			return "", "", time.Time{}, user.ErrNotFound
		}
	}
	raw := newCode()
	expires = s.now().Add(7 * 24 * time.Hour)
	inviteID := newID()
	err = s.Centers.SaveInvite(ctx, port.Invite{
		ID:              inviteID,
		FitnessCenterID: centerID,
		Email:           account.Email,
		Role:            "manager",
		BranchIDs:       branchIDs,
		CodeHash:        hashCode(raw),
		ExpiresAt:       expires.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	return inviteID, raw, expires, nil
}

// ReplaceManagerBranches sets the branches a manager covers.
func (s *Service) ReplaceManagerBranches(ctx context.Context, userID, managerUserID string, branchIDs []string) error {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return err
	}
	centerID := actor.Memberships[0].FitnessCenterID
	if err := membership.CanManagePeople(actor, centerID, "", membership.RoleManager); err != nil {
		return err
	}
	if len(branchIDs) == 0 {
		return fmt.Errorf("branch")
	}
	for _, branchID := range branchIDs {
		ok, err := s.Centers.BranchBelongs(ctx, centerID, branchID)
		if err != nil {
			return err
		}
		if !ok {
			return user.ErrNotFound
		}
	}
	return s.Centers.ReplaceManagerBranches(ctx, centerID, managerUserID, branchIDs)
}

func (s *Service) actor(ctx context.Context, userID string) (membership.Actor, error) {
	rows, err := s.Users.Memberships(ctx, userID)
	if err != nil {
		return membership.Actor{}, err
	}
	if len(rows) == 0 {
		return membership.Actor{}, user.ErrUnauthorized
	}
	out := make([]membership.Membership, 0, len(rows))
	for _, row := range rows {
		item := membership.Membership{
			FitnessCenterID: row.FitnessCenterID,
			Role:            membership.Role(row.Role),
			Status:          membership.Status(row.Status),
			BranchID:        row.BranchID,
		}
		for _, branch := range row.Branches {
			item.BranchIDs = append(item.BranchIDs, branch.ID)
		}
		out = append(out, item)
	}
	return membership.Actor{UserID: userID, Memberships: out}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func filterBranches(all []port.Branch, allowed map[string]bool) []port.Branch {
	out := []port.Branch{}
	for _, branch := range all {
		if allowed[branch.ID] {
			out = append(out, branch)
		}
	}
	return out
}

func hashCode(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newCode() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
