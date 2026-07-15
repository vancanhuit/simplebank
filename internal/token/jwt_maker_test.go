package token

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "01234567890123456789012345678901"

func TestJWTMaker(t *testing.T) {
	maker, err := NewJWTMaker(testSecret)
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
	maker, _ := NewJWTMaker(testSecret)
	token, _, _ := maker.CreateToken("alice", "depositor", -time.Minute)
	_, err := maker.VerifyToken(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestJWTMakerInvalidAlg(t *testing.T) {
	payload, _ := NewPayload("alice", "depositor", time.Minute)
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, payload)
	signed, _ := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	maker, _ := NewJWTMaker(testSecret)
	if _, err := maker.VerifyToken(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestJWTMakerWrongSecret(t *testing.T) {
	maker, _ := NewJWTMaker(testSecret)
	token, _, _ := maker.CreateToken("alice", "depositor", time.Minute)
	other, _ := NewJWTMaker("abcdefghijabcdefghijabcdefghij12")
	if _, err := other.VerifyToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestNewJWTMakerShortKey(t *testing.T) {
	if _, err := NewJWTMaker("short"); err == nil {
		t.Fatal("want error for key shorter than 32 chars")
	}
}
