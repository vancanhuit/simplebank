package api

import (
	"errors"
	"net/http"
	"testing"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestToHTTPStatus(t *testing.T) {
	cases := map[error]int{
		store.ErrRecordNotFound:          http.StatusNotFound,
		store.ErrUniqueViolation:         http.StatusConflict,
		store.ErrForeignKeyViolation:     http.StatusConflict,
		store.ErrInsufficientBalance:     http.StatusUnprocessableEntity,
		token.ErrExpiredToken:            http.StatusUnauthorized,
		token.ErrInvalidToken:            http.StatusUnauthorized,
		errors.New("some unknown error"): http.StatusInternalServerError,
	}
	for err, want := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			if got := toHTTPStatus(err); got != want {
				t.Errorf("got %d want %d", got, want)
			}
		})
	}
}

func TestClientMessage(t *testing.T) {
	cases := map[error]string{
		store.ErrRecordNotFound:          "resource not found",
		store.ErrUniqueViolation:         "resource already exists",
		store.ErrForeignKeyViolation:     "related resource not found",
		store.ErrInsufficientBalance:     "insufficient balance",
		token.ErrExpiredToken:            "token has expired",
		token.ErrInvalidToken:            "token is invalid",
		errors.New("some unknown error"): "request failed",
	}
	for err, want := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			if got := clientMessage(err); got != want {
				t.Errorf("got %q want %q", got, want)
			}
		})
	}
}
