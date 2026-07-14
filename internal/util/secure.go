package util

import (
	"crypto/rand"
	"encoding/hex"
)

// SecureToken returns a cryptographically secure random token encoded as a
// hex string. n is the number of random bytes; the returned string is 2*n chars.
func SecureToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
