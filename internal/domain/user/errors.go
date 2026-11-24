package user

import "errors"

var (
	// ErrUnauthorized is a bad login, a bad token, or a wrong current password.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrEmailInUse is a second account with the same email.
	ErrEmailInUse = errors.New("email already in use")
	// ErrNotFound is a missing account.
	ErrNotFound = errors.New("not found")
	// ErrResetInvalid is a used, expired, or unknown reset token.
	ErrResetInvalid = errors.New("reset is not valid")
)

// ValidAccountType reports whether register may create this label.
// A fitness center uses its own route.
func ValidAccountType(value AccountType) bool {
	switch value {
	case AccountClient, AccountTrainer, AccountManager:
		return true
	default:
		return false
	}
}
