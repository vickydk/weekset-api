package home

import (
	"context"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/membership"
	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// Service builds the first tab for the caller's role.
type Service struct {
	Centers  port.CenterStore
	Users    port.UserRepository
	Schedule port.ScheduleStore
}

// Card is one dashboard. Workout fields are never set.
type Card struct {
	Role          string
	Question      string
	CenterName    string
	BranchName    string
	EmptyTitle    string
	EmptyMessage  string
	PrimaryAction string
	Attention     []Attention
	Sessions      []Session
	Branches      []BranchLoad
}

// Attention is one thing a manager or the center should do.
type Attention struct {
	Label  string
	Action string
}

// Session is a timetable row. It has no note, sets, reps, or load.
type Session struct {
	ID          string
	Time        string
	ClientName  string
	TrainerName string
	BranchName  string
	Status      string
}

// BranchLoad is one branch on the fitness-center dashboard.
type BranchLoad struct {
	Name             string
	SessionsToday    int
	RequestsWaiting  int
	ClientsWithoutPT int
	HasManager       bool
}

// Home returns the cards for this caller. A wrong center or branch is not found.
func (s *Service) Home(ctx context.Context, userID string, headers membership.Headers) (Card, error) {
	rows, err := s.Users.Memberships(ctx, userID)
	if err != nil {
		return Card{}, err
	}
	actor, err := actorFrom(userID, rows)
	if err != nil {
		return Card{}, err
	}
	scope, err := membership.Resolve(actor, headers, membership.BodyClaim{})
	if err != nil {
		return Card{}, err
	}
	name := centerLabel(rows, scope.FitnessCenterID)
	switch scope.Role {
	case membership.RoleClient:
		card := Card{
			Role: string(scope.Role), Question: "What do I do next?",
			CenterName: name, BranchName: branchLabel(rows, scope),
			EmptyTitle: "No session today", EmptyMessage: "See open hours.",
			PrimaryAction: "See open hours",
			Attention:     []Attention{}, Sessions: []Session{}, Branches: []BranchLoad{},
		}
		return s.withToday(ctx, scope, card)
	case membership.RoleTrainer:
		rules, err := s.Centers.CountTrainerRules(ctx, scope.FitnessCenterID, userID)
		if err != nil {
			return Card{}, err
		}
		action := "Set hours"
		if rules > 0 {
			action = "Your hours are open."
		}
		card := Card{
			Role: string(scope.Role), Question: "Who needs me today?",
			CenterName: name, BranchName: branchLabel(rows, scope),
			EmptyTitle: "No one booked.", EmptyMessage: action, PrimaryAction: action,
			Attention: []Attention{}, Sessions: []Session{}, Branches: []BranchLoad{},
		}
		return s.withToday(ctx, scope, card)
	case membership.RoleManager:
		return s.manager(ctx, scope, name)
	case membership.RoleCenterAdmin:
		return s.center(ctx, scope, name)
	default:
		return Card{}, &membership.AccessError{Code: membership.CodeForbidden, Message: "forbidden"}
	}
}

func (s *Service) manager(ctx context.Context, scope membership.Scope, centerName string) (Card, error) {
	people, err := s.Centers.ListPeople(ctx, scope.FitnessCenterID, scope.BranchID, false)
	if err != nil {
		return Card{}, err
	}
	invites, err := s.Centers.ListOpenInvites(ctx, scope.FitnessCenterID)
	if err != nil {
		return Card{}, err
	}
	attention := []Attention{}
	for _, person := range people {
		if person.Role == string(membership.RoleClient) && person.TrainerName == "" {
			attention = append(attention, Attention{Label: person.DisplayName + " has no PT", Action: "Assign"})
		}
	}
	for _, invite := range invites {
		if contains(invite.BranchIDs, scope.BranchID) && invite.Role != string(membership.RoleManager) {
			attention = append(attention, Attention{Label: invite.Email + " has not accepted", Action: "Invite"})
		}
	}
	branch := ""
	for _, person := range people {
		if person.BranchName != "" {
			branch = person.BranchName
			break
		}
	}
	if branch == "" {
		branches, err := s.Centers.ListBranches(ctx, scope.FitnessCenterID)
		if err != nil {
			return Card{}, err
		}
		for _, item := range branches {
			if item.ID == scope.BranchID {
				branch = item.Name
			}
		}
	}
	card := Card{
		Role: "manager", Question: "What is stuck at this branch?",
		CenterName: centerName, BranchName: branch,
		EmptyTitle: "No one at this branch yet", EmptyMessage: "Invite.",
		PrimaryAction: "Invite",
		Attention:     attention, Sessions: []Session{}, Branches: []BranchLoad{},
	}
	if len(attention) > 0 {
		card.PrimaryAction = attention[0].Action
		card.EmptyTitle = ""
	}
	if len(attention) > 2 {
		card.Attention = attention[:2]
	}
	return s.withToday(ctx, scope, card)
}

func (s *Service) center(ctx context.Context, scope membership.Scope, centerName string) (Card, error) {
	branches, err := s.Centers.ListBranches(ctx, scope.FitnessCenterID)
	if err != nil {
		return Card{}, err
	}
	people, err := s.Centers.ListPeople(ctx, scope.FitnessCenterID, "", true)
	if err != nil {
		return Card{}, err
	}
	loads := make([]BranchLoad, 0, len(branches))
	needsManager := ""
	for _, branch := range branches {
		load := BranchLoad{Name: branch.Name}
		hasManager := false
		for _, person := range people {
			if person.BranchID != branch.ID && person.BranchName != branch.Name {
				continue
			}
			if person.Role == string(membership.RoleManager) {
				hasManager = true
			}
			if person.Role == string(membership.RoleClient) && person.TrainerName == "" {
				load.ClientsWithoutPT++
			}
		}
		load.HasManager = hasManager
		if !hasManager && needsManager == "" {
			needsManager = branch.Name
		}
		loads = append(loads, load)
	}
	card := Card{
		Role: "center_admin", Question: "Which branch needs an owner?",
		CenterName: centerName, PrimaryAction: "Invite a manager",
		Attention: []Attention{}, Sessions: []Session{}, Branches: loads,
	}
	if needsManager != "" {
		card.EmptyTitle = "Invite a manager for " + needsManager + "."
		card.EmptyMessage = card.EmptyTitle
	}
	return s.withToday(ctx, scope, card)
}

func (s *Service) withToday(ctx context.Context, scope membership.Scope, card Card) (Card, error) {
	if s.Schedule == nil {
		return card, nil
	}
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	rows, err := s.Schedule.ListBookings(ctx, scope.FitnessCenterID, "", "", from, from.AddDate(0, 0, 1))
	if err != nil {
		return Card{}, err
	}
	sessions := []Session{}
	for _, row := range rows {
		switch scope.Role {
		case membership.RoleClient:
			if row.ClientUserID != scope.UserID {
				continue
			}
		case membership.RoleTrainer:
			if row.TrainerUserID != scope.UserID {
				continue
			}
		case membership.RoleManager:
			if row.BranchID != scope.BranchID {
				continue
			}
		}
		sessions = append(sessions, Session{
			ID: row.ID, Time: row.StartsAt.UTC().Format("15:04"),
			ClientName: row.ClientName, TrainerName: row.TrainerName,
			BranchName: card.BranchName, Status: row.Status,
		})
	}
	card.Sessions = sessions
	return card, nil
}

func actorFrom(userID string, rows []port.MembershipView) (membership.Actor, error) {
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

func centerLabel(rows []port.MembershipView, centerID string) string {
	for _, row := range rows {
		if row.FitnessCenterID == centerID && row.CenterName != "" {
			return row.CenterName
		}
	}
	return ""
}

func branchLabel(rows []port.MembershipView, scope membership.Scope) string {
	for _, row := range rows {
		if row.FitnessCenterID != scope.FitnessCenterID {
			continue
		}
		if row.BranchName != "" {
			return row.BranchName
		}
		for _, branch := range row.Branches {
			if branch.ID == scope.BranchID {
				return branch.Name
			}
		}
	}
	return ""
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
