package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrExpiredToken = errors.New("token has expired")
	ErrInvalidToken = errors.New("token is invalid")
)

type Payload struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	jwt.RegisteredClaims
}

func NewPayload(username, role string, duration time.Duration) (*Payload, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &Payload{
		ID:       id,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        id.String(),
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}, nil
}

type Maker interface {
	CreateToken(username, role string, duration time.Duration) (string, *Payload, error)
	VerifyToken(token string) (*Payload, error)
}
