package people

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// ErrInviteInvalid is a wrong, used, or expired code.
var ErrInviteInvalid = errors.New("invite is not valid")

// ErrNotWaitingTrainer is an email that is not a trainer without a branch.
var ErrNotWaitingTrainer = errors.New("account is not waiting as a trainer")

// ErrNotWaitingClient is an email that is not a client who can join this center.
var ErrNotWaitingClient = errors.New("account is not waiting as a client")

// Service invites people and keeps the roster.
type Service struct {
	Centers port.CenterStore
	Users   port.UserRepository
	Now     func() time.Time
}

// Page is the roster and the codes that have not been accepted.
type Page struct {
	People  []port.Person
	Invites []PendingInvite
}

// PendingInvite is a code that has not been accepted.
type PendingInvite struct {
	Email      string
	Role       string
	BranchName string
	ExpiresAt  string
}

// Invite creates a one-time code for a trainer or a client at one branch.
func (s *Service) Invite(ctx context.Context, userID, email, role, branchID string, headers membership.Headers) (id, code string, expires time.Time, err error) {
	if role != string(membership.RoleTrainer) && role != string(membership.RoleClient) {
		return "", "", time.Time{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	scope, err := membership.Resolve(actor, headers, membership.BodyClaim{})
	if err != nil {
		return "", "", time.Time{}, err
	}
	target := membership.Role(role)
	if err := membership.CanManagePeople(actor, scope.FitnessCenterID, branchFor(scope, branchID), target); err != nil {
		return "", "", time.Time{}, err
	}
	branchID, err = s.branchForInvite(ctx, scope, branchID)
	if err != nil {
		return "", "", time.Time{}, err
	}
	account, err := s.waitingAccount(ctx, email, scope.FitnessCenterID, target)
	if err != nil {
		return "", "", time.Time{}, err
	}
	raw := newCode()
	expires = s.now().Add(7 * 24 * time.Hour)
	inviteID := newID()
	err = s.Centers.SaveInvite(ctx, port.Invite{
		ID: inviteID, FitnessCenterID: scope.FitnessCenterID, Email: account.Email,
		Role: role, BranchIDs: []string{branchID}, CodeHash: hashCode(raw),
		ExpiresAt: expires.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	return inviteID, raw, expires, nil
}

// Accept turns a code into a membership for the invited account.
func (s *Service) Accept(ctx context.Context, code string) error {
	invite, err := s.Centers.FindInvite(ctx, hashCode(strings.TrimSpace(code)))
	if err != nil || invite.Accepted || s.expired(invite.ExpiresAt) || len(invite.BranchIDs) == 0 {
		return ErrInviteInvalid
	}
	account, err := s.Users.FindByEmail(ctx, invite.Email)
	if err != nil {
		return ErrInviteInvalid
	}
	rows, err := s.Users.Memberships(ctx, account.ID)
	if err != nil {
		return err
	}
	next := membership.Membership{
		FitnessCenterID: invite.FitnessCenterID,
		Role:            membership.Role(invite.Role),
		Status:          membership.StatusActive,
	}
	draft := port.MembershipDraft{
		ID: newID(), UserID: account.ID, FitnessCenterID: invite.FitnessCenterID, Role: invite.Role,
	}
	if invite.Role == string(membership.RoleManager) {
		draft.ManagerBranchIDs = invite.BranchIDs
	} else {
		next.BranchID = invite.BranchIDs[0]
		draft.BranchID = invite.BranchIDs[0]
	}
	if err := membership.AcceptMembership(toDomain(rows), next); err != nil {
		return trainerBranch(invite.Role, err)
	}
	if err := s.Centers.AcceptInvite(ctx, invite.ID, draft); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return ErrInviteInvalid
		}
		return trainerBranch(invite.Role, err)
	}
	return nil
}

// Assign sets the client's trainer at the selected branch.
func (s *Service) Assign(ctx context.Context, userID, clientUserID, trainerUserID string, headers membership.Headers) error {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return err
	}
	scope, err := membership.Resolve(actor, headers, membership.BodyClaim{})
	if err != nil {
		return err
	}
	client, err := s.Centers.FindMember(ctx, scope.FitnessCenterID, clientUserID)
	if err != nil {
		return err
	}
	if client.Role != string(membership.RoleClient) || client.Status != string(membership.StatusActive) {
		return user.ErrNotFound
	}
	if scope.Role == membership.RoleManager && client.BranchID != scope.BranchID {
		return user.ErrNotFound
	}
	if err := membership.CanManagePeople(actor, scope.FitnessCenterID, client.BranchID, membership.RoleClient); err != nil {
		return err
	}
	trainer, err := s.Centers.FindMember(ctx, scope.FitnessCenterID, trainerUserID)
	if err != nil {
		return &membership.AccessError{Code: membership.CodeValidation, Message: "trainer is not at this branch"}
	}
	if trainer.Role != string(membership.RoleTrainer) || trainer.Status != string(membership.StatusActive) || trainer.BranchID != client.BranchID {
		return &membership.AccessError{Code: membership.CodeValidation, Message: "trainer is not at this branch"}
	}
	return s.Centers.AssignTrainer(ctx, port.Assignment{
		ID: newID(), FitnessCenterID: scope.FitnessCenterID, BranchID: client.BranchID,
		ClientUserID: client.UserID, TrainerUserID: trainer.UserID,
		ClientMembershipID: client.MembershipID, TrainerMembershipID: trainer.MembershipID,
	})
}

// People lists the roster the caller may manage.
func (s *Service) People(ctx context.Context, userID string, headers membership.Headers) (Page, error) {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return Page{}, err
	}
	scope, err := membership.Resolve(actor, headers, membership.BodyClaim{})
	if err != nil {
		return Page{}, err
	}
	if scope.Role != membership.RoleManager && scope.Role != membership.RoleCenterAdmin {
		return Page{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
	branchID := ""
	includeManagers := scope.Role == membership.RoleCenterAdmin
	if scope.Role == membership.RoleManager {
		branchID = scope.BranchID
	}
	people, err := s.Centers.ListPeople(ctx, scope.FitnessCenterID, branchID, includeManagers)
	if err != nil {
		return Page{}, err
	}
	if people == nil {
		people = []port.Person{}
	}
	open, err := s.Centers.ListOpenInvites(ctx, scope.FitnessCenterID)
	if err != nil {
		return Page{}, err
	}
	names := map[string]string{}
	branches, err := s.Centers.ListBranches(ctx, scope.FitnessCenterID)
	if err != nil {
		return Page{}, err
	}
	for _, branch := range branches {
		names[branch.ID] = branch.Name
	}
	pending := []PendingInvite{}
	for _, invite := range open {
		if scope.Role == membership.RoleManager && !contains(invite.BranchIDs, scope.BranchID) {
			continue
		}
		if scope.Role == membership.RoleManager && invite.Role == string(membership.RoleManager) {
			continue
		}
		pending = append(pending, PendingInvite{
			Email: invite.Email, Role: invite.Role,
			BranchName: branchNames(names, invite.BranchIDs), ExpiresAt: invite.ExpiresAt,
		})
	}
	return Page{People: people, Invites: pending}, nil
}

// Suspend blocks one membership. A client's other centers stay as they are.
func (s *Service) Suspend(ctx context.Context, userID, targetUserID string, headers membership.Headers) error {
	return s.setStatus(ctx, userID, targetUserID, string(membership.StatusSuspended), headers)
}

// Restore makes a suspended membership active again.
func (s *Service) Restore(ctx context.Context, userID, targetUserID string, headers membership.Headers) error {
	return s.setStatus(ctx, userID, targetUserID, string(membership.StatusActive), headers)
}

// Remove deletes the membership and the trainer assignment. Bookings and logs stay.
func (s *Service) Remove(ctx context.Context, userID, targetUserID string, headers membership.Headers) error {
	member, err := s.managed(ctx, userID, targetUserID, headers)
	if err != nil {
		return err
	}
	return s.Centers.DeleteMembership(ctx, member.centerID, targetUserID)
}

func (s *Service) setStatus(ctx context.Context, userID, targetUserID, status string, headers membership.Headers) error {
	member, err := s.managed(ctx, userID, targetUserID, headers)
	if err != nil {
		return err
	}
	return s.Centers.SetMembershipStatus(ctx, member.centerID, targetUserID, status)
}

type managedMember struct {
	centerID string
}

func (s *Service) managed(ctx context.Context, userID, targetUserID string, headers membership.Headers) (managedMember, error) {
	actor, err := s.actor(ctx, userID)
	if err != nil {
		return managedMember{}, err
	}
	scope, err := membership.Resolve(actor, headers, membership.BodyClaim{})
	if err != nil {
		return managedMember{}, err
	}
	target, err := s.Centers.FindMember(ctx, scope.FitnessCenterID, targetUserID)
	if err != nil {
		return managedMember{}, err
	}
	branchID := target.BranchID
	if target.Role == string(membership.RoleManager) {
		branchID = scope.BranchID
	}
	if scope.Role == membership.RoleManager && target.Role != string(membership.RoleManager) && target.BranchID != scope.BranchID {
		return managedMember{}, user.ErrNotFound
	}
	if err := membership.CanManagePeople(actor, scope.FitnessCenterID, branchID, membership.Role(target.Role)); err != nil {
		return managedMember{}, err
	}
	return managedMember{centerID: scope.FitnessCenterID}, nil
}

func (s *Service) branchForInvite(ctx context.Context, scope membership.Scope, branchID string) (string, error) {
	if scope.Role == membership.RoleManager {
		if branchID != "" && branchID != scope.BranchID {
			return "", user.ErrNotFound
		}
		return scope.BranchID, nil
	}
	ok, err := s.Centers.BranchBelongs(ctx, scope.FitnessCenterID, branchID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", user.ErrNotFound
	}
	return branchID, nil
}

func (s *Service) waitingAccount(ctx context.Context, email, centerID string, role membership.Role) (user.User, error) {
	account, err := s.Users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	waiting := ErrNotWaitingClient
	if role == membership.RoleTrainer {
		waiting = ErrNotWaitingTrainer
	}
	if err != nil || user.AccountType(role) != account.AccountType {
		return user.User{}, waiting
	}
	rows, err := s.Users.Memberships(ctx, account.ID)
	if err != nil {
		return user.User{}, err
	}
	next := membership.Membership{
		FitnessCenterID: centerID,
		Role:            role,
		Status:          membership.StatusActive,
		BranchID:        "pending",
	}
	if err := membership.AcceptMembership(toDomain(rows), next); err != nil {
		return user.User{}, trainerBranch(string(role), err)
	}
	return account, nil
}

func trainerBranch(role string, err error) error {
	if role == string(membership.RoleTrainer) && (errors.Is(err, membership.ErrAlreadyInCenter) || errors.Is(err, membership.ErrTrainerHasBranch)) {
		return membership.ErrTrainerHasBranch
	}
	return err
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

func (s *Service) expired(raw string) bool {
	expires, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	return !expires.After(s.now())
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func toDomain(rows []port.MembershipView) []membership.Membership {
	out := make([]membership.Membership, 0, len(rows))
	for _, row := range rows {
		out = append(out, membership.Membership{
			FitnessCenterID: row.FitnessCenterID,
			Role:            membership.Role(row.Role),
			Status:          membership.Status(row.Status),
			BranchID:        row.BranchID,
		})
	}
	return out
}

func branchFor(scope membership.Scope, branchID string) string {
	if scope.Role == membership.RoleManager {
		return scope.BranchID
	}
	return branchID
}

func branchNames(names map[string]string, ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if name := names[id]; name != "" {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
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
