package token

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "01234567890123456789012345678901"

func TestJWTMaker(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	maker, err := NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := maker.CreateToken("alice", "depositor", -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maker.VerifyToken(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestJWTMakerInvalidAlg(t *testing.T) {
	t.Parallel()
	payload, err := NewPayload("alice", "depositor", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodNone, payload)
	signed, err := jwtToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	maker, err := NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maker.VerifyToken(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestJWTMakerWrongSecret(t *testing.T) {
	t.Parallel()
	maker, err := NewJWTMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := maker.CreateToken("alice", "depositor", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewJWTMaker("abcdefghijabcdefghijabcdefghij12")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.VerifyToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestNewJWTMakerShortKey(t *testing.T) {
	t.Parallel()
	if _, err := NewJWTMaker("short"); err == nil {
		t.Fatal("want error for key shorter than 32 chars")
	}
}
