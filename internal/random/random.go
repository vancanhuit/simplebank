package random

import (
	"math/rand/v2"
	"strings"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

func String(n int) string {
	var sb strings.Builder
	for range n {
		sb.WriteByte(alphabet[rand.IntN(len(alphabet))])
	}
	return sb.String()
}

func Owner() string {
	return String(6)
}
