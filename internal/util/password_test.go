package util

import (
	"errors"
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
	if err := CheckPassword("wrong", hashed); !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
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

// bcrypt rejects inputs longer than 72 bytes; HashPassword must surface that
// error rather than silently truncating.
func TestHashPasswordTooLong(t *testing.T) {
	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := HashPassword(string(long)); !errors.Is(err, bcrypt.ErrPasswordTooLong) {
		t.Fatalf("want ErrPasswordTooLong for >72-byte input, got %v", err)
	}
}
