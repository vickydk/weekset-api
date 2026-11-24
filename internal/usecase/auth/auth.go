// Package auth is registration, login, and the locked-screen session.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vickydk/weekset-api/internal/domain/user"
	"github.com/vickydk/weekset-api/internal/platform/password"
	"github.com/vickydk/weekset-api/internal/platform/token"
	"github.com/vickydk/weekset-api/internal/port"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 30 * 24 * time.Hour
	resetTTL   = 24 * time.Hour
)

// Service registers people and issues tokens. It does not open a menu.
type Service struct {
	Users  port.UserRepository
	Tokens port.RefreshStore
	Resets port.ResetStore
	Mailer port.Mailer
	Hasher password.Hasher
	Issuer token.Issuer
	Now    func() time.Time
}

// Session is the token pair plus the /v1/me body.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Profile      Profile
}

// Profile is GET /v1/me.
type Profile struct {
	ID          string
	Email       string
	DisplayName string
	AccountType user.AccountType
	Memberships []port.MembershipView
}

// RegisterInput is a client, PT, or manager. A center uses another route.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	AccountType user.AccountType
}

// Register creates the account and no membership.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Session, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !user.ValidAccountType(in.AccountType) {
		return Session{}, fmt.Errorf("account type")
	}
	hash, err := s.Hasher.Hash(in.Password)
	if err != nil {
		return Session{}, err
	}
	account := user.User{
		ID:           newID(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		AccountType:  in.AccountType,
	}
	if err := s.Users.Create(ctx, account); err != nil {
		return Session{}, err
	}
	return s.issue(ctx, account, newID())
}

// Login accepts a known email and password. Both failures look the same.
func (s *Service) Login(ctx context.Context, email, plain string) (Session, error) {
	account, err := s.Users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return Session{}, user.ErrUnauthorized
	}
	if err := s.Hasher.Compare(account.PasswordHash, plain); err != nil {
		return Session{}, user.ErrUnauthorized
	}
	return s.issue(ctx, account, newID())
}

// Refresh rotates the presented token. A reused token revokes its family.
func (s *Service) Refresh(ctx context.Context, raw string) (Session, error) {
	row, err := s.Tokens.FindRefresh(ctx, hashToken(raw))
	if err != nil {
		return Session{}, user.ErrUnauthorized
	}
	now := s.now()
	if row.RevokedAt != nil {
		_ = s.Tokens.RevokeFamily(ctx, row.FamilyID, now)
		return Session{}, user.ErrUnauthorized
	}
	if !row.ExpiresAt.After(now) {
		return Session{}, user.ErrUnauthorized
	}
	if err := s.Tokens.RevokeRefresh(ctx, row.ID, now); err != nil {
		return Session{}, err
	}
	account, err := s.Users.FindByID(ctx, row.UserID)
	if err != nil {
		return Session{}, user.ErrUnauthorized
	}
	return s.issue(ctx, account, row.FamilyID)
}

// Logout revokes one refresh token.
func (s *Service) Logout(ctx context.Context, raw string) error {
	row, err := s.Tokens.FindRefresh(ctx, hashToken(raw))
	if err != nil {
		return nil
	}
	return s.Tokens.RevokeRefresh(ctx, row.ID, s.now())
}

// Forgot always succeeds. A known email gets one reset token.
func (s *Service) Forgot(ctx context.Context, email string) error {
	account, err := s.Users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil
	}
	raw := newID() + newID()
	reset := port.PasswordReset{
		ID:        newID(),
		UserID:    account.ID,
		TokenHash: hashToken(raw),
		ExpiresAt: s.now().Add(resetTTL),
	}
	if err := s.Resets.SaveReset(ctx, reset); err != nil {
		return err
	}
	return s.Mailer.SendPasswordReset(ctx, account.Email, raw)
}

// ResetPassword consumes one token and revokes that person's refresh tokens.
func (s *Service) ResetPassword(ctx context.Context, raw, plain string) error {
	row, err := s.Resets.FindReset(ctx, hashToken(raw))
	if err != nil || row.UsedAt != nil || !row.ExpiresAt.After(s.now()) {
		return user.ErrResetInvalid
	}
	hash, err := s.Hasher.Hash(plain)
	if err != nil {
		return err
	}
	if err := s.Users.UpdatePassword(ctx, row.UserID, hash); err != nil {
		return err
	}
	now := s.now()
	if err := s.Resets.MarkResetUsed(ctx, row.ID, now); err != nil {
		return err
	}
	return s.Tokens.RevokeUserRefresh(ctx, row.UserID, now)
}

// ChangePassword checks the current password, then revokes refresh tokens.
func (s *Service) ChangePassword(ctx context.Context, userID, current, next string) error {
	account, err := s.Users.FindByID(ctx, userID)
	if err != nil {
		return user.ErrUnauthorized
	}
	if err := s.Hasher.Compare(account.PasswordHash, current); err != nil {
		return user.ErrUnauthorized
	}
	hash, err := s.Hasher.Hash(next)
	if err != nil {
		return err
	}
	if err := s.Users.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.Tokens.RevokeUserRefresh(ctx, userID, s.now())
}

// Me is the signed-in account and its memberships.
func (s *Service) Me(ctx context.Context, userID string) (Profile, error) {
	account, err := s.Users.FindByID(ctx, userID)
	if err != nil {
		return Profile{}, user.ErrUnauthorized
	}
	return s.profile(ctx, account)
}

func (s *Service) issue(ctx context.Context, account user.User, familyID string) (Session, error) {
	now := s.now()
	access, err := s.Issuer.Sign(account.ID, now)
	if err != nil {
		return Session{}, err
	}
	raw := newID() + newID()
	err = s.Tokens.SaveRefresh(ctx, port.RefreshToken{
		ID:        newID(),
		UserID:    account.ID,
		FamilyID:  familyID,
		TokenHash: hashToken(raw),
		ExpiresAt: now.Add(refreshTTL),
	})
	if err != nil {
		return Session{}, err
	}
	profile, err := s.profile(ctx, account)
	if err != nil {
		return Session{}, err
	}
	return Session{
		AccessToken:  access,
		RefreshToken: raw,
		ExpiresIn:    int(accessTTL.Seconds()),
		Profile:      profile,
	}, nil
}

func (s *Service) profile(ctx context.Context, account user.User) (Profile, error) {
	rows, err := s.Users.Memberships(ctx, account.ID)
	if err != nil {
		return Profile{}, err
	}
	if rows == nil {
		rows = []port.MembershipView{}
	}
	return Profile{
		ID:          account.ID,
		Email:       account.Email,
		DisplayName: account.DisplayName,
		AccountType: account.AccountType,
		Memberships: rows,
	}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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

// Subject reads an access token.
func (s *Service) Subject(raw string) (string, error) {
	id, err := s.Issuer.Subject(raw, s.now())
	if err != nil {
		return "", user.ErrUnauthorized
	}
	return id, nil
}

// IsUnauthorized reports a login or token failure.
func IsUnauthorized(err error) bool {
	return errors.Is(err, user.ErrUnauthorized)
}
