package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTMaker(t *testing.T) {
	maker, err := NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, payload, err := maker.CreateToken("alice", "depositor", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || payload == nil {
		t.Fatal("expected token and payload")
	}
	got, err := maker.VerifyToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "alice" || got.Role != "depositor" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestJWTMakerExpired(t *testing.T) {
	maker, _ := NewJWTMaker("01234567890123456789012345678901")
	token, _, _ := maker.CreateToken("alice", "depositor", -time.Minute)
	_, err := maker.VerifyToken(token)
	if err != ErrExpiredToken {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestJWTMakerInvalidAlg(t *testing.T) {
	payload, _ := NewPayload("alice", "depositor", time.Minute)
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, payload)
	signed, _ := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	maker, _ := NewJWTMaker("01234567890123456789012345678901")
	if _, err := maker.VerifyToken(signed); err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}
