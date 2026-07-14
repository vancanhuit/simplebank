package util

import "testing"

func TestSecureToken(t *testing.T) {
	a, err := SecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || b == "" {
		t.Fatal("token should not be empty")
	}
	if a == b {
		t.Fatal("two secure tokens should differ")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}
