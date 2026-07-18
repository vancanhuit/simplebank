package secret

import "testing"

func TestToken(t *testing.T) {
	t.Parallel()
	a, err := Token(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Token(32)
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

// Token(0) currently returns an empty string and no error. Pin that behavior so
// a future change to reject n<=0 is a deliberate decision.
func TestTokenZero(t *testing.T) {
	t.Parallel()
	tok, err := Token(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "" {
		t.Fatalf("want empty token for n=0, got %q", tok)
	}
}
