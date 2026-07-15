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
	// hex encoding of n bytes yields 2*n chars.
	if len(a) != 64 {
		t.Fatalf("token length = %d, want 64", len(a))
	}
}
