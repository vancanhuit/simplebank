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

type Type string

const (
	Access  Type = "access"
	Refresh Type = "refresh"
)

type Payload struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	TokenType Type      `json:"token_type"`
	Nonce     string    `json:"nonce,omitempty"`
	jwt.RegisteredClaims
	expectedType Type
}

func NewExpectedPayload(expectedType Type) *Payload {
	return &Payload{expectedType: expectedType}
}

func (p Payload) Validate() error {
	if p.expectedType != "" && p.TokenType != p.expectedType {
		return ErrInvalidToken
	}
	if p.TokenType != Access && p.TokenType != Refresh {
		return ErrInvalidToken
	}
	return nil
}

func NewPayload(username, role string, tokenType Type, duration time.Duration) (*Payload, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return NewPayloadWithID(id, username, role, tokenType, duration), nil
}

func NewPayloadWithID(id uuid.UUID, username, role string, tokenType Type, duration time.Duration) *Payload {
	now := time.Now()
	return &Payload{
		ID:        id,
		Username:  username,
		Role:      role,
		TokenType: tokenType,
		Nonce:     uuid.NewString(),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        id.String(),
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}
}

type Maker interface {
	CreateToken(username, role string, tokenType Type, duration time.Duration) (string, *Payload, error)
	CreateTokenWithID(id uuid.UUID, username, role string, tokenType Type, duration time.Duration) (string, *Payload, error)
	VerifyToken(raw string, expectedType Type) (*Payload, error)
}
