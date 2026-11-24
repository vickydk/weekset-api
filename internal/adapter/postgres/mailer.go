package postgres

import "context"

// LogMailer keeps the reset token off the logs. Delivery waits for a mailer.
type LogMailer struct{}

// SendPasswordReset accepts the token and does not record it.
func (LogMailer) SendPasswordReset(context.Context, string, string) error {
	return nil
}
