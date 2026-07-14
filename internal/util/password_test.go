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
