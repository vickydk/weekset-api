package memory

import (
	"context"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func (s *Store) CreateCenter(_ context.Context, draft port.CenterDraft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byEmail[draft.Email]; ok {
		return user.ErrEmailInUse
	}
	s.users[draft.UserID] = user.User{
		ID:           draft.UserID,
		Email:        draft.Email,
		PasswordHash: draft.PasswordHash,
		DisplayName:  draft.DisplayName,
		AccountType:  user.AccountCenter,
	}
	s.byEmail[draft.Email] = draft.UserID
	s.members[draft.UserID] = []port.MembershipView{{
		Role:            "center_admin",
		Status:          "active",
		FitnessCenterID: draft.CenterID,
		CenterName:      draft.CenterName,
		Branches: []port.BranchView{{
			ID:   draft.BranchID,
			Name: draft.BranchName,
		}},
	}}
	s.branches = append(s.branches, port.Branch{
		ID: draft.BranchID, FitnessCenterID: draft.CenterID, Name: draft.BranchName,
	})
	return nil
}

func (s *Store) AddBranch(_ context.Context, centerID, branchID, name string) (port.Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	branch := port.Branch{ID: branchID, FitnessCenterID: centerID, Name: name}
	s.branches = append(s.branches, branch)
	return branch, nil
}

func (s *Store) ListBranches(_ context.Context, centerID string) ([]port.Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []port.Branch
	for _, branch := range s.branches {
		if branch.FitnessCenterID == centerID {
			out = append(out, branch)
		}
	}
	if out == nil {
		out = []port.Branch{}
	}
	return out, nil
}

func (s *Store) RenameBranch(_ context.Context, centerID, branchID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.branches {
		if s.branches[i].ID == branchID && s.branches[i].FitnessCenterID == centerID {
			s.branches[i].Name = name
			return nil
		}
	}
	return user.ErrNotFound
}

func (s *Store) RenameCenter(_ context.Context, centerID, name, timezone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.centers = append(s.centers, storedCenter{ID: centerID, Name: name, Timezone: timezone})
	for userID, rows := range s.members {
		for i := range rows {
			if rows[i].FitnessCenterID == centerID && name != "" {
				rows[i].CenterName = name
			}
		}
		s.members[userID] = rows
	}
	return nil
}

func (s *Store) SaveInvite(_ context.Context, invite port.Invite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invites = append(s.invites, invite)
	return nil
}

func (s *Store) ReplaceManagerBranches(_ context.Context, centerID, userID string, branchIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.members[userID]
	for i := range rows {
		if rows[i].FitnessCenterID != centerID || rows[i].Role != "manager" {
			continue
		}
		rows[i].Branches = nil
		for _, id := range branchIDs {
			rows[i].Branches = append(rows[i].Branches, port.BranchView{ID: id, Name: id})
		}
		s.members[userID] = rows
		return nil
	}
	return user.ErrNotFound
}

func (s *Store) BranchBelongs(_ context.Context, centerID, branchID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, branch := range s.branches {
		if branch.ID == branchID && branch.FitnessCenterID == centerID {
			return true, nil
		}
	}
	return false, nil
}

type storedCenter struct {
	ID       string
	Name     string
	Timezone string
}
