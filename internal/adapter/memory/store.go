// Package memory is an in-process store for tests. The server uses Postgres.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/port"
)

// Store is the account, the refresh family, and password resets.
type Store struct {
	mu          sync.Mutex
	users       map[string]user.User
	byEmail     map[string]string
	refresh     []port.RefreshToken
	resets      []port.PasswordReset
	members     map[string][]port.MembershipView
	branches    []port.Branch
	centers     []storedCenter
	invites     []port.Invite
	memberIDs   map[string]string
	assignments map[string]port.Assignment
	zones       map[string]string
	rules       []port.AvailabilityRule
	exceptions  []port.AvailabilityException
	held        []heldHour
	bookings    []port.Booking
	series      []port.Series
	skips       []string
	notices     []port.Notification
	lastMail    string
}

// NewStore is empty.
func NewStore() *Store {
	return &Store{
		users:       map[string]user.User{},
		byEmail:     map[string]string{},
		members:     map[string][]port.MembershipView{},
		memberIDs:   map[string]string{},
		assignments: map[string]port.Assignment{},
		zones:       map[string]string{},
	}
}

func (s *Store) Create(_ context.Context, account user.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byEmail[account.Email]; ok {
		return user.ErrEmailInUse
	}
	s.users[account.ID] = account
	s.byEmail[account.Email] = account.ID
	return nil
}

func (s *Store) FindByEmail(_ context.Context, email string) (user.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[email]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return s.users[id], nil
}

func (s *Store) FindByID(_ context.Context, id string) (user.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.users[id]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return account, nil
}

func (s *Store) UpdatePassword(_ context.Context, id, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.users[id]
	if !ok {
		return user.ErrNotFound
	}
	account.PasswordHash = passwordHash
	s.users[id] = account
	return nil
}

func (s *Store) Memberships(_ context.Context, userID string) ([]port.MembershipView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]port.MembershipView(nil), s.members[userID]...), nil
}

// PutMemberships is how a test gives an account a center.
func (s *Store) PutMemberships(userID string, rows []port.MembershipView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.members[userID] = rows
	for _, row := range rows {
		s.memberIDs[memberKey(userID, row.FitnessCenterID)] = userID + ":" + row.FitnessCenterID
	}
}

func (s *Store) SaveRefresh(_ context.Context, token port.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh = append(s.refresh, token)
	return nil
}

func (s *Store) FindRefresh(_ context.Context, tokenHash string) (port.RefreshToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, token := range s.refresh {
		if token.TokenHash == tokenHash {
			return token, nil
		}
	}
	return port.RefreshToken{}, user.ErrNotFound
}

func (s *Store) RevokeRefresh(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.refresh {
		if s.refresh[i].ID == id && s.refresh[i].RevokedAt == nil {
			s.refresh[i].RevokedAt = &at
		}
	}
	return nil
}

func (s *Store) RevokeFamily(_ context.Context, familyID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.refresh {
		if s.refresh[i].FamilyID == familyID && s.refresh[i].RevokedAt == nil {
			s.refresh[i].RevokedAt = &at
		}
	}
	return nil
}

func (s *Store) RevokeUserRefresh(_ context.Context, userID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.refresh {
		if s.refresh[i].UserID == userID && s.refresh[i].RevokedAt == nil {
			s.refresh[i].RevokedAt = &at
		}
	}
	return nil
}

func (s *Store) SaveReset(_ context.Context, reset port.PasswordReset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets = append(s.resets, reset)
	return nil
}

func (s *Store) FindReset(_ context.Context, tokenHash string) (port.PasswordReset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, reset := range s.resets {
		if reset.TokenHash == tokenHash {
			return reset, nil
		}
	}
	return port.PasswordReset{}, user.ErrNotFound
}

func (s *Store) MarkResetUsed(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.resets {
		if s.resets[i].ID == id {
			s.resets[i].UsedAt = &at
		}
	}
	return nil
}

// SendPasswordReset records the latest token so a test can complete the reset.
func (s *Store) SendPasswordReset(_ context.Context, _, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMail = token
	return nil
}

// LastResetToken is the token the mailer would have sent.
func (s *Store) LastResetToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastMail
}
