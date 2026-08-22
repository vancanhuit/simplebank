package secret

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Token returns a cryptographically secure random token encoded as a hex
// string. n is the number of random bytes; the returned string is 2*n chars.
func Token(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Digest returns a deterministic SHA-256 digest suitable for comparing
// high-entropy tokens without storing the bearer credential itself.
func Digest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
