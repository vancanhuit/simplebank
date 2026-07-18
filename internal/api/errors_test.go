package api

import (
	"errors"
	"net/http"
	"testing"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestToHTTPStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want int
	}{
		{store.ErrRecordNotFound, http.StatusNotFound},
		{store.ErrUniqueViolation, http.StatusConflict},
		{store.ErrForeignKeyViolation, http.StatusConflict},
		{store.ErrInsufficientBalance, http.StatusUnprocessableEntity},
		{token.ErrExpiredToken, http.StatusUnauthorized},
		{token.ErrInvalidToken, http.StatusUnauthorized},
		{errors.New("some unknown error"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			t.Parallel()
			if got := toHTTPStatus(tc.err); got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestClientMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{store.ErrRecordNotFound, "resource not found"},
		{store.ErrUniqueViolation, "resource already exists"},
		{store.ErrForeignKeyViolation, "related resource not found"},
		{store.ErrInsufficientBalance, "insufficient balance"},
		{token.ErrExpiredToken, "token has expired"},
		{token.ErrInvalidToken, "token is invalid"},
		{errors.New("some unknown error"), "request failed"},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			t.Parallel()
			if got := clientMessage(tc.err); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
