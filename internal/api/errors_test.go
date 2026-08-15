package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

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
		{store.ErrBalanceLimitExceeded, http.StatusUnprocessableEntity, "destination balance exceeds the supported limit"},
		{store.ErrIdempotencyConflict, http.StatusConflict, "idempotency key conflicts with an existing transfer"},
		{store.ErrInvalidSession, http.StatusUnauthorized, "invalid session"},
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

func TestErrorHandlerPreservesErrorSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "custom HTTP error",
			err:         echo.NewHTTPError(http.StatusTeapot, "custom message"),
			wantStatus:  http.StatusTeapot,
			wantMessage: "custom message",
		},
		{
			name:        "Echo status error",
			err:         echo.ErrMethodNotAllowed,
			wantStatus:  http.StatusMethodNotAllowed,
			wantMessage: http.StatusText(http.StatusMethodNotAllowed),
		},
		{
			name:        "domain error",
			err:         store.ErrInvalidSession,
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "invalid session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(req, rec)

			errorHandler(ctx, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["error"] != tt.wantMessage {
				t.Fatalf("message = %q, want %q", body["error"], tt.wantMessage)
			}
		})
	}
}
