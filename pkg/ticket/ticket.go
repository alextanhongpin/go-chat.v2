package ticket

import (
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
)

type Issuer interface {
	Issue(user string) (string, error)
	Verify(token string) (string, error)
}

type Ticket struct {
	secret   []byte
	validity time.Duration
}

func New(secret []byte, validity time.Duration) *Ticket {
	return &Ticket{
		secret:   secret,
		validity: validity,
	}
}

func (t *Ticket) Issue(user string) (string, error) {
	claims := &jwt.StandardClaims{
		ExpiresAt: time.Now().Add(t.validity).Unix(),
		Subject:   user,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(t.secret)
	return ss, err
}

func (t *Ticket) Verify(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return t.secret, nil
	})
	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims["sub"].(string), nil
	}
	return "", err
}
