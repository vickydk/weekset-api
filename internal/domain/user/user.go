// Package user is the account. The account type is a label for the invite.
// It does not authorize a route.
package user

// AccountType is chosen at registration.
type AccountType string

const (
	AccountClient  AccountType = "client"
	AccountTrainer AccountType = "trainer"
	AccountManager AccountType = "manager"
	AccountCenter  AccountType = "center"
)

// User is the login. Role lives on the membership.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	AccountType  AccountType
}
