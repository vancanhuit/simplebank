package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func toHTTPStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrRecordNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrUniqueViolation), errors.Is(err, store.ErrForeignKeyViolation):
		return http.StatusConflict
	case errors.Is(err, store.ErrInsufficientBalance):
		return http.StatusUnprocessableEntity
	case errors.Is(err, token.ErrExpiredToken), errors.Is(err, token.ErrInvalidToken):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func errorHandler(c *echo.Context, err error) {
	if r, _ := echo.UnwrapResponse(c.Response()); r != nil && r.Committed {
		return
	}

	var he *echo.HTTPError
	if errors.As(err, &he) {
		_ = c.JSON(he.StatusCode(), map[string]string{"error": he.Message})
		return
	}

	status := toHTTPStatus(err)
	message := "internal server error"
	if status != http.StatusInternalServerError {
		message = err.Error()
	} else {
		c.Logger().Error("request failed", "error", err)
	}
	_ = c.JSON(status, map[string]string{"error": message})
}
