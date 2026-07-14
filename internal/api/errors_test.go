package api

import (
	"net/http"
	"testing"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestToHTTPStatus(t *testing.T) {
	cases := map[error]int{
		store.ErrRecordNotFound:      http.StatusNotFound,
		store.ErrUniqueViolation:     http.StatusConflict,
		store.ErrForeignKeyViolation: http.StatusConflict,
		store.ErrInsufficientBalance: http.StatusUnprocessableEntity,
		token.ErrExpiredToken:        http.StatusUnauthorized,
		token.ErrInvalidToken:        http.StatusUnauthorized,
	}
	for err, want := range cases {
		if got := toHTTPStatus(err); got != want {
			t.Errorf("%v: got %d want %d", err, got, want)
		}
	}
}
