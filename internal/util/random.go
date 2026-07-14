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
