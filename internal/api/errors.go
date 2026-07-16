package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

// errorCatalog maps each domain sentinel error to its HTTP status and
// client-safe message. Keeping the mapping in one place means adding a new
// error edits a single row instead of two parallel switches.
var errorCatalog = []struct {
	is      error
	status  int
	message string
}{
	{store.ErrRecordNotFound, http.StatusNotFound, "resource not found"},
	{store.ErrUniqueViolation, http.StatusConflict, "resource already exists"},
	{store.ErrForeignKeyViolation, http.StatusConflict, "related resource not found"},
	{store.ErrInsufficientBalance, http.StatusUnprocessableEntity, "insufficient balance"},
	{token.ErrExpiredToken, http.StatusUnauthorized, "token has expired"},
	{token.ErrInvalidToken, http.StatusUnauthorized, "token is invalid"},
}

func toHTTPStatus(err error) int {
	for _, e := range errorCatalog {
		if errors.Is(err, e.is) {
			return e.status
		}
	}
	return http.StatusInternalServerError
}

func clientMessage(err error) string {
	for _, e := range errorCatalog {
		if errors.Is(err, e.is) {
			return e.message
		}
	}
	return "request failed"
}

func errorHandler(c *echo.Context, err error) {
	if r, _ := echo.UnwrapResponse(c.Response()); r != nil && r.Committed {
		return
	}

	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		_ = c.JSON(he.StatusCode(), map[string]string{"error": he.Message})
		return
	}

	status := toHTTPStatus(err)
	message := "internal server error"
	if status != http.StatusInternalServerError {
		message = clientMessage(err)
	}
	// The request logger middleware logs the error once with full request
	// context (status, path, request_id); logging here too would duplicate it.
	_ = c.JSON(status, map[string]string{"error": message})
}
