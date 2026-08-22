package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

const refreshCookieName = "simplebank_refresh"

func (s *Server) setRefreshCookie(c *echo.Context, raw string, expiresAt time.Time) {
	//nolint:gosec // Secure is required for HTTPS and configurable for local HTTP development.
	c.SetCookie(&http.Cookie{
		Name:     refreshCookieName,
		Value:    raw,
		Path:     "/api/v1",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   s.config.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearRefreshCookie(c *echo.Context) {
	//nolint:gosec // Secure is required for HTTPS and configurable for local HTTP development.
	c.SetCookie(&http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.SessionCookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}
