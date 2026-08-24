package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

type errorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

type apiError struct {
	status  int
	code    string
	message string
}

func (e apiError) Error() string { return e.message }

func newAPIError(status int, code, message string) error {
	return apiError{status: status, code: code, message: message}
}

// errorCatalog maps each domain sentinel error to its HTTP status, stable code,
// and client-safe message.
var errorCatalog = []struct {
	is      error
	status  int
	code    string
	message string
}{
	{store.ErrRecordNotFound, http.StatusNotFound, "not_found", "resource not found"},
	{store.ErrUsernameExists, http.StatusConflict, "username_exists", "username already exists"},
	{store.ErrEmailExists, http.StatusConflict, "email_exists", "email already exists"},
	{store.ErrUniqueViolation, http.StatusConflict, "already_exists", "resource already exists"},
	{store.ErrForeignKeyViolation, http.StatusConflict, "related_not_found", "related resource not found"},
	{store.ErrInsufficientBalance, http.StatusUnprocessableEntity, "insufficient_balance", "insufficient balance"},
	{store.ErrBalanceLimitExceeded, http.StatusUnprocessableEntity, "destination_balance_limit_exceeded", "destination balance exceeds the supported limit"},
	{store.ErrCurrencyMismatch, http.StatusBadRequest, "currency_mismatch", "currency mismatch"},
	{store.ErrDailyLimitExceeded, http.StatusUnprocessableEntity, "daily_limit_exceeded", "daily transfer limit exceeded"},
	{store.ErrNumericOutOfRange, http.StatusUnprocessableEntity, "amount_too_large", "amount too large"},
	{store.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict", "idempotency key conflicts with an existing transfer"},
	{store.ErrInvalidSession, http.StatusUnauthorized, "invalid_session", "invalid session"},
	{token.ErrExpiredToken, http.StatusUnauthorized, "token_expired", "token has expired"},
	{token.ErrInvalidToken, http.StatusUnauthorized, "token_invalid", "token is invalid"},
}

// lookupError resolves a domain error in a single pass over the catalog.
func lookupError(err error) apiError {
	for _, e := range errorCatalog {
		if errors.Is(err, e.is) {
			return apiError{status: e.status, code: e.code, message: e.message}
		}
	}
	return apiError{
		status:  http.StatusInternalServerError,
		code:    "internal_error",
		message: "internal server error",
	}
}

func statusError(status int) apiError {
	code := "request_failed"
	switch status {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusUnauthorized:
		code = "unauthorized"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusMethodNotAllowed:
		code = "method_not_allowed"
	case http.StatusConflict:
		code = "conflict"
	case http.StatusRequestEntityTooLarge:
		code = "payload_too_large"
	case http.StatusUnsupportedMediaType:
		code = "unsupported_media_type"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	}
	return apiError{status: status, code: code, message: http.StatusText(status)}
}

func errorHandler(c *echo.Context, err error) {
	if r, _ := echo.UnwrapResponse(c.Response()); r != nil && r.Committed {
		return
	}

	if apiErr, ok := errors.AsType[apiError](err); ok {
		_ = c.JSON(apiErr.status, errorResponse{Code: apiErr.code, Error: apiErr.message})
		return
	}

	apiErr := lookupError(err)
	if apiErr.code != "internal_error" {
		_ = c.JSON(apiErr.status, errorResponse{Code: apiErr.code, Error: apiErr.message})
		return
	}

	if status := echo.StatusCode(err); status != 0 {
		apiErr = statusError(status)
	}
	// The request logger middleware logs the error once with full request
	// context (status, path, request_id); logging here too would duplicate it.
	_ = c.JSON(apiErr.status, errorResponse{Code: apiErr.code, Error: apiErr.message})
}
