# Task 9: JWT token maker

**Files:**
- Create: `internal/token/maker.go`
- Create: `internal/token/jwt_maker.go`
- Test: `internal/token/jwt_maker_test.go`

## Produces
- `type Payload struct { ID uuid.UUID; Username string; Role string; jwt.RegisteredClaims }`
- `func NewPayload(username, role string, duration time.Duration) (*Payload, error)`
- `var ErrExpiredToken; var ErrInvalidToken`
- `type Maker interface { CreateToken(username, role string, duration time.Duration) (string, *Payload, error); VerifyToken(token string) (*Payload, error) }`
- `type JWTMaker struct { secretKey string }`, `func NewJWTMaker(secretKey string) (*JWTMaker, error)` (errors if `len(secretKey) < 32`).

## Step 1: Write failing test `internal/token/jwt_maker_test.go`
```go
package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTMaker(t *testing.T) {
	maker, err := NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, payload, err := maker.CreateToken("alice", "depositor", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || payload == nil {
		t.Fatal("expected token and payload")
	}
	got, err := maker.VerifyToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "alice" || got.Role != "depositor" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestJWTMakerExpired(t *testing.T) {
	maker, _ := NewJWTMaker("01234567890123456789012345678901")
	token, _, _ := maker.CreateToken("alice", "depositor", -time.Minute)
	_, err := maker.VerifyToken(token)
	if err != ErrExpiredToken {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestJWTMakerInvalidAlg(t *testing.T) {
	payload, _ := NewPayload("alice", "depositor", time.Minute)
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, payload)
	signed, _ := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	maker, _ := NewJWTMaker("01234567890123456789012345678901")
	if _, err := maker.VerifyToken(signed); err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}
```

## Step 2: Run test, verify FAIL
`go test ./internal/token/ -v` → FAIL (undefined types).

## Step 3: Write `internal/token/maker.go`
```go
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
```

## Step 4: Write `internal/token/jwt_maker.go`
```go
package token

import (
	"errors"
	"time"

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

func (m *JWTMaker) CreateToken(username, role string, duration time.Duration) (string, *Payload, error) {
	payload, err := NewPayload(username, role, duration)
	if err != nil {
		return "", nil, err
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	signed, err := jwtToken.SignedString([]byte(m.secretKey))
	return signed, payload, err
}

func (m *JWTMaker) VerifyToken(token string) (*Payload, error) {
	keyFunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(m.secretKey), nil
	}
	parsed, err := jwt.ParseWithClaims(token, &Payload{}, keyFunc,
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
```

## Step 5: Run tests, verify PASS
`go test ./internal/token/ -v` → PASS (all three).

## Step 6: Commit
```bash
git add internal/token/
git commit -m "feat: add JWT token maker with access and refresh payloads"
```

## Global Constraints
- `golang-jwt/jwt/v5` (added in Task 1).
- JWT secret min length 32 enforced in `NewJWTMaker`.
- HS256 only; reject other algorithms (the `WithValidMethods` + HMAC check).
- Never log tokens/secrets.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-9-report.md`. Return only: status, commit hash(es), one-line test summary, concerns.
