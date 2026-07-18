package password

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/vancanhuit/simplebank/internal/random"
)

func TestHashAndCheck(t *testing.T) {
	t.Parallel()
	pw := random.String(8)

	hashed, err := Hash(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := Check(pw, hashed); err != nil {
		t.Fatalf("check should pass: %v", err)
	}
	if err := Check("wrong", hashed); !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Fatalf("check should fail with mismatch, got %v", err)
	}

	hashed2, err := Hash(pw)
	if err != nil {
		t.Fatalf("hash2: %v", err)
	}
	if hashed == hashed2 {
		t.Fatal("hashes should differ due to salt")
	}
}

// bcrypt rejects inputs longer than 72 bytes; Hash must surface that error
// rather than silently truncating.
func TestHashTooLong(t *testing.T) {
	t.Parallel()
	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := Hash(string(long)); !errors.Is(err, bcrypt.ErrPasswordTooLong) {
		t.Fatalf("want ErrPasswordTooLong for >72-byte input, got %v", err)
	}
}
