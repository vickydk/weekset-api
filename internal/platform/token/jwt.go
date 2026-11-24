package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer signs access tokens that name the user and nothing else.
type Issuer struct {
	Secret []byte
	TTL    time.Duration
}

// AccessClaims is the JWT body. Role is not in here.
type AccessClaims struct {
	jwt.RegisteredClaims
}

// Sign returns a signed access token for userID.
func (i Issuer) Sign(userID string, now time.Time) (string, error) {
	claims := AccessClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.TTL)),
	}}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.Secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// Subject returns the user id when the token is still valid.
func (i Issuer) Subject(raw string, now time.Time) (string, error) {
	parsed, err := jwt.ParseWithClaims(raw, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return i.Secret, nil
	}, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		return "", err
	}
	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid || claims.Subject == "" {
		return "", fmt.Errorf("invalid access token")
	}
	return claims.Subject, nil
}
