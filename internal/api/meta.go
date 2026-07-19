package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/vancanhuit/simplebank/internal/config"
)

// transferLimits exposes the per-currency transfer ceilings so the SPA can
// validate amounts against the same limits the API enforces, instead of
// hard-coding its own copy. It is public (no credentials reveal only policy
// numbers) and returns an empty object when no limits are configured.
func (s *Server) transferLimits(c *echo.Context) error {
	limits := s.config.TransferLimits
	if limits == nil {
		limits = map[string]config.CurrencyLimit{}
	}
	return c.JSON(http.StatusOK, limits)
}
