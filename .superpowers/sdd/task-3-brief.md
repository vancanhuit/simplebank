# Task 3: Utility helpers (password hashing, random, currency)

**Files:**
- Create: `internal/util/password.go`
- Create: `internal/util/random.go`
- Create: `internal/util/currency.go`
- Test: `internal/util/password_test.go`
- Test: `internal/util/currency_test.go`

## Produces
- `func HashPassword(password string) (string, error)`
- `func CheckPassword(password, hashedPassword string) error` (nil if match)
- `func RandomString(n int) string`
- `func RandomOwner() string`
- `func IsSupportedCurrency(currency string) bool` (USD, EUR, VND)
- `const (USD = "USD"; EUR = "EUR"; VND = "VND")`

## Step 1: Write failing tests

`internal/util/password_test.go`:
```go
package util

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPassword(t *testing.T) {
	password := RandomString(8)

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := CheckPassword(password, hashed); err != nil {
		t.Fatalf("check should pass: %v", err)
	}
	if err := CheckPassword("wrong", hashed); err != bcrypt.ErrMismatchedHashAndPassword {
		t.Fatalf("check should fail with mismatch, got %v", err)
	}

	hashed2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash2: %v", err)
	}
	if hashed == hashed2 {
		t.Fatal("hashes should differ due to salt")
	}
}
```

`internal/util/currency_test.go`:
```go
package util

import "testing"

func TestIsSupportedCurrency(t *testing.T) {
	if !IsSupportedCurrency(USD) {
		t.Error("USD should be supported")
	}
	if IsSupportedCurrency("XYZ") {
		t.Error("XYZ should not be supported")
	}
}
```

## Step 2: Run tests, verify FAIL
`go test ./internal/util/ -v` → FAIL (undefined functions).

## Step 3: Write implementations

`internal/util/password.go`:
```go
package util

import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func CheckPassword(password, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
```

`internal/util/random.go`:
```go
package util

import (
	"math/rand"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

func RandomString(n int) string {
	var sb strings.Builder
	for range n {
		sb.WriteByte(alphabet[rand.Intn(len(alphabet))])
	}
	return sb.String()
}

func RandomOwner() string {
	return RandomString(6)
}
```

`internal/util/currency.go`:
```go
package util

const (
	USD = "USD"
	EUR = "EUR"
	VND = "VND"
)

func IsSupportedCurrency(currency string) bool {
	switch currency {
	case USD, EUR, VND:
		return true
	}
	return false
}
```

## Step 4: Run tests, verify PASS
`go test ./internal/util/ -v` → PASS.

## Step 5: Commit
```bash
git add internal/util/
git commit -m "feat: add password hashing, random, and currency helpers"
```

## Global Constraints
- Module path `github.com/vancanhuit/simplebank`, Go `1.26.5`.
- bcrypt cost must be `>= 12` (use 12).
- Never log secrets/passwords.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-3-report.md`. Return only: status, commit hash(es), one-line test summary, concerns.
