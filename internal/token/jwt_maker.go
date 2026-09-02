package token

import (
	"errors"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
)

type JWTMaker struct {
	secretKey string
}

func NewJWTMaker(secretKey string) (*JWTMaker, error) {
	if len(secretKey) < 32 {
		return nil, errors.New("secret key must be at least 32 characters")
	}
	return &JWTMaker{secretKey: secretKey}, nil
}

func (m *JWTMaker) CreateToken(username, role string, tokenType Type, duration time.Duration) (string, *Payload, error) {
	return m.signPayload(NewPayload(username, role, tokenType, duration))
}

func (m *JWTMaker) CreateTokenWithID(id uuid.UUID, username, role string, tokenType Type, duration time.Duration) (string, *Payload, error) {
	payload := NewPayloadWithID(id, username, role, tokenType, duration)
	return m.signPayload(payload)
}

func (m *JWTMaker) signPayload(payload *Payload) (string, *Payload, error) {
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	signed, err := jwtToken.SignedString([]byte(m.secretKey))
	return signed, payload, err
}

func (m *JWTMaker) VerifyToken(token string, expectedType Type) (*Payload, error) {
	keyFunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(m.secretKey), nil
	}
	parsed, err := jwt.ParseWithClaims(token, NewExpectedPayload(expectedType), keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	payload, ok := parsed.Claims.(*Payload)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return payload, nil
}
