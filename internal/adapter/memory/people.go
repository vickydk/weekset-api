package memory

import (
	"context"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

func (s *Store) FindInvite(_ context.Context, codeHash string) (port.Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, invite := range s.invites {
		if invite.CodeHash == codeHash {
			return invite, nil
		}
	}
	return port.Invite{}, user.ErrNotFound
}

func (s *Store) AcceptInvite(_ context.Context, inviteID string, draft port.MembershipDraft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.invites {
		if s.invites[i].ID != inviteID {
			continue
		}
		if s.invites[i].Accepted {
			return user.ErrNotFound
		}
		s.invites[i].Accepted = true
		row := port.MembershipView{
			Role:            draft.Role,
			Status:          "active",
			FitnessCenterID: draft.FitnessCenterID,
			CenterName:      s.centerNameLocked(draft.FitnessCenterID),
			BranchID:        draft.BranchID,
			BranchName:      s.branchNameLocked(draft.BranchID),
		}
		for _, branchID := range draft.ManagerBranchIDs {
			row.Branches = append(row.Branches, port.BranchView{ID: branchID, Name: s.branchNameLocked(branchID)})
		}
		s.members[draft.UserID] = append(s.members[draft.UserID], row)
		s.memberIDs[memberKey(draft.UserID, draft.FitnessCenterID)] = draft.ID
		return nil
	}
	return user.ErrNotFound
}

func (s *Store) ListPeople(_ context.Context, centerID, branchID string, includeManagers bool) ([]port.Person, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.Person{}
	for userID, rows := range s.members {
		account := s.users[userID]
		for _, row := range rows {
			if row.FitnessCenterID != centerID {
				continue
			}
			if row.Role == "manager" {
				if !includeManagers {
					continue
				}
				for _, branch := range row.Branches {
					if branchID != "" && branch.ID != branchID {
						continue
					}
					out = append(out, port.Person{
						UserID: userID, DisplayName: account.DisplayName, Email: account.Email,
						Role: row.Role, Status: row.Status, BranchID: branch.ID, BranchName: branch.Name,
					})
				}
				continue
			}
			if row.Role != "client" && row.Role != "trainer" {
				continue
			}
			if branchID != "" && row.BranchID != branchID {
				continue
			}
			out = append(out, port.Person{
				UserID: userID, DisplayName: account.DisplayName, Email: account.Email,
				Role: row.Role, Status: row.Status, BranchID: row.BranchID, BranchName: row.BranchName,
				TrainerName: s.trainerNameLocked(centerID, userID),
			})
		}
	}
	return out, nil
}

func (s *Store) ListOpenInvites(_ context.Context, centerID string) ([]port.Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []port.Invite{}
	for _, invite := range s.invites {
		if invite.FitnessCenterID == centerID && !invite.Accepted {
			out = append(out, invite)
		}
	}
	return out, nil
}

func (s *Store) FindMember(_ context.Context, centerID, userID string) (port.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.users[userID]
	if !ok {
		return port.Member{}, user.ErrNotFound
	}
	for _, row := range s.members[userID] {
		if row.FitnessCenterID != centerID {
			continue
		}
		return port.Member{
			MembershipID: s.memberIDs[memberKey(userID, centerID)],
			UserID:       userID,
			Role:         row.Role,
			Status:       row.Status,
			BranchID:     row.BranchID,
			DisplayName:  account.DisplayName,
			Email:        account.Email,
		}, nil
	}
	return port.Member{}, user.ErrNotFound
}

func (s *Store) SetMembershipStatus(_ context.Context, centerID, userID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.members[userID]
	for i := range rows {
		if rows[i].FitnessCenterID != centerID {
			continue
		}
		rows[i].Status = status
		s.members[userID] = rows
		return nil
	}
	return user.ErrNotFound
}

func (s *Store) DeleteMembership(_ context.Context, centerID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.members[userID]
	next := rows[:0]
	found := false
	for _, row := range rows {
		if row.FitnessCenterID == centerID {
			found = true
			continue
		}
		next = append(next, row)
	}
	if !found {
		return user.ErrNotFound
	}
	s.members[userID] = next
	delete(s.memberIDs, memberKey(userID, centerID))
	for key, assignment := range s.assignments {
		if assignment.FitnessCenterID == centerID && (assignment.ClientUserID == userID || assignment.TrainerUserID == userID) {
			delete(s.assignments, key)
		}
	}
	return nil
}

func (s *Store) AssignTrainer(_ context.Context, draft port.Assignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := memberKey(draft.ClientUserID, draft.FitnessCenterID)
	s.assignments[key] = draft
	rows := s.members[draft.ClientUserID]
	for i := range rows {
		if rows[i].FitnessCenterID == draft.FitnessCenterID {
			rows[i].TrainerUserID = draft.TrainerUserID
		}
	}
	s.members[draft.ClientUserID] = rows
	return nil
}

func (s *Store) CountTrainerRules(_ context.Context, centerID, trainerID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, rule := range s.rules {
		if rule.FitnessCenterID == centerID && rule.TrainerUserID == trainerID {
			count++
		}
	}
	return count, nil
}

func memberKey(userID, centerID string) string {
	return userID + "|" + centerID
}

func (s *Store) centerNameLocked(centerID string) string {
	for _, rows := range s.members {
		for _, row := range rows {
			if row.FitnessCenterID == centerID && row.CenterName != "" {
				return row.CenterName
			}
		}
	}
	return ""
}

func (s *Store) branchNameLocked(branchID string) string {
	for _, branch := range s.branches {
		if branch.ID == branchID {
			return branch.Name
		}
	}
	return ""
}

func (s *Store) trainerNameLocked(centerID, clientUserID string) string {
	assignment, ok := s.assignments[memberKey(clientUserID, centerID)]
	if !ok {
		return ""
	}
	return s.users[assignment.TrainerUserID].DisplayName
}
