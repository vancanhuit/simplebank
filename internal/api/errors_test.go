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
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"not found", store.ErrRecordNotFound, http.StatusNotFound, "not_found", "resource not found"},
		{"username exists", store.ErrUsernameExists, http.StatusConflict, "username_exists", "username already exists"},
		{"email exists", store.ErrEmailExists, http.StatusConflict, "email_exists", "email already exists"},
		{"unique violation", store.ErrUniqueViolation, http.StatusConflict, "already_exists", "resource already exists"},
		{"foreign key violation", store.ErrForeignKeyViolation, http.StatusConflict, "related_not_found", "related resource not found"},
		{"insufficient balance", store.ErrInsufficientBalance, http.StatusUnprocessableEntity, "insufficient_balance", "insufficient balance"},
		{"balance limit exceeded", store.ErrBalanceLimitExceeded, http.StatusUnprocessableEntity, "destination_balance_limit_exceeded", "destination balance exceeds the supported limit"},
		{"currency mismatch", store.ErrCurrencyMismatch, http.StatusBadRequest, "currency_mismatch", "currency mismatch"},
		{"daily limit exceeded", store.ErrDailyLimitExceeded, http.StatusUnprocessableEntity, "daily_limit_exceeded", "daily transfer limit exceeded"},
		{"numeric out of range", store.ErrNumericOutOfRange, http.StatusUnprocessableEntity, "amount_too_large", "amount too large"},
		{"idempotency conflict", store.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict", "idempotency key conflicts with an existing transfer"},
		{"invalid session", store.ErrInvalidSession, http.StatusUnauthorized, "invalid_session", "invalid session"},
		{"expired token", token.ErrExpiredToken, http.StatusUnauthorized, "token_expired", "token has expired"},
		{"invalid token", token.ErrInvalidToken, http.StatusUnauthorized, "token_invalid", "token is invalid"},
		{"unknown", errors.New("database password leaked"), http.StatusInternalServerError, "internal_error", "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lookupError(tt.err)
			if got.status != tt.wantStatus {
				t.Errorf("status: got %d want %d", got.status, tt.wantStatus)
			}
			if got.code != tt.wantCode {
				t.Errorf("code: got %q want %q", got.code, tt.wantCode)
			}
			if got.message != tt.wantMessage {
				t.Errorf("message: got %q want %q", got.message, tt.wantMessage)
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
		wantCode    string
		wantMessage string
	}{
		{
			name:        "typed API error",
			err:         newAPIError(http.StatusBadRequest, "invalid_account_id", "invalid account id"),
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_account_id",
			wantMessage: "invalid account id",
		},
		{
			name:        "unrestricted HTTP error",
			err:         echo.NewHTTPError(http.StatusTeapot, "sensitive detail"),
			wantStatus:  http.StatusTeapot,
			wantCode:    "request_failed",
			wantMessage: http.StatusText(http.StatusTeapot),
		},
		{
			name:        "Echo status error",
			err:         echo.ErrMethodNotAllowed,
			wantStatus:  http.StatusMethodNotAllowed,
			wantCode:    "method_not_allowed",
			wantMessage: http.StatusText(http.StatusMethodNotAllowed),
		},
		{
			name:        "domain error",
			err:         store.ErrInvalidSession,
			wantStatus:  http.StatusUnauthorized,
			wantCode:    "invalid_session",
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
			var body errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Error != tt.wantMessage {
				t.Errorf("message = %q, want %q", body.Error, tt.wantMessage)
			}
		})
	}
}
