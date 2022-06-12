package ticket

import (
	"errors"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
)

var ErrTicketExpired = errors.New("ticket: expired")

type Issuer interface {
	Issue(subject string) (string, error)
	Verify(token string) (string, error)
}

type Ticket struct {
	secret    []byte
	expiresIn time.Duration
}

func New(secret []byte, expiresIn time.Duration) *Ticket {
	return &Ticket{
		secret:    secret,
		expiresIn: expiresIn,
	}
}

func (t *Ticket) Issue(subject string) (string, error) {
	claims := &jwt.StandardClaims{
		ExpiresAt: time.Now().Add(t.expiresIn).Unix(),
		Subject:   subject,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	ss, err := token.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("ticket: failed to sign string: %w", err)
	}

	return ss, nil
}

func (t *Ticket) Verify(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("ticket: unexpected signing method: %v", token.Header["alg"])
		}

		return t.secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("ticket: failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*jwt.StandardClaims); ok && token.Valid {
		return claims.Subject, nil
	}

	return "", ErrTicketExpired
}
