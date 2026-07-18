package api

import (
	"errors"
	"net/http"
	"testing"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestLookupError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err         error
		wantStatus  int
		wantMessage string
	}{
		{store.ErrRecordNotFound, http.StatusNotFound, "resource not found"},
		{store.ErrUniqueViolation, http.StatusConflict, "resource already exists"},
		{store.ErrForeignKeyViolation, http.StatusConflict, "related resource not found"},
		{store.ErrInsufficientBalance, http.StatusUnprocessableEntity, "insufficient balance"},
		{token.ErrExpiredToken, http.StatusUnauthorized, "token has expired"},
		{token.ErrInvalidToken, http.StatusUnauthorized, "token is invalid"},
		{errors.New("some unknown error"), http.StatusInternalServerError, "request failed"},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			t.Parallel()
			status, message := lookupError(tc.err)
			if status != tc.wantStatus {
				t.Errorf("status: got %d want %d", status, tc.wantStatus)
			}
			if message != tc.wantMessage {
				t.Errorf("message: got %q want %q", message, tc.wantMessage)
			}
		})
	}
}
