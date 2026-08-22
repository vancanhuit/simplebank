package api

import (
	"net/http"
	"net/url"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v5"
	"github.com/labstack/echo/v5"

	"github.com/vancanhuit/simplebank/internal/token"
)

const (
	authContextKey     = "user"
	roleDepositor      = "depositor"
	headerSecFetchSite = "Sec-Fetch-Site"
)

func (s *Server) authMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey: []byte(s.config.JWTSecret),
		ContextKey: authContextKey,
		NewClaimsFunc: func(c *echo.Context) jwt.Claims {
			return token.NewExpectedPayload(token.Access)
		},
	})
}

func authPayload(c *echo.Context) (*token.Payload, error) {
	jwtToken, err := echo.ContextGet[*jwt.Token](c, authContextKey)
	if err != nil {
		return nil, echo.ErrUnauthorized
	}
	payload, ok := jwtToken.Claims.(*token.Payload)
	if !ok {
		return nil, echo.ErrUnauthorized
	}
	return payload, nil
}

// authorizeOwner enforces that the authenticated caller owns the resource. It is
// the single seam for resource-ownership decisions, so the policy lives in one
// place instead of being re-derived in every handler.
func authorizeOwner(payload *token.Payload, owner string) error {
	if payload.Username != owner {
		return echo.NewHTTPError(http.StatusForbidden, "you do not have access to this resource")
	}
	return nil
}

// sameOrigin rejects browser requests from hostile origins before a refresh
// cookie can be used. Non-browser clients without Origin/Fetch Metadata remain
// supported.
func (s *Server) sameOrigin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		if site := r.Header.Get(headerSecFetchSite); site != "" && site != "same-origin" && site != "none" {
			return echo.NewHTTPError(http.StatusForbidden, "cross-origin request denied")
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			return next(c)
		}
		got, err := url.Parse(origin)
		if err != nil || got.Scheme == "" || got.Host == "" || got.Path != "" {
			return echo.NewHTTPError(http.StatusForbidden, "cross-origin request denied")
		}
		expectedScheme := "http"
		expectedHost := forwardedHost(r.Host, r.Header)
		if r.TLS != nil {
			expectedScheme = "https"
		} else if forwardedScheme := r.Header.Get(echo.HeaderXForwardedProto); forwardedScheme == "http" || forwardedScheme == "https" {
			expectedScheme = forwardedScheme
		}
		if s.config.PublicBaseURL != "" {
			expected, parseErr := url.Parse(s.config.PublicBaseURL)
			if parseErr == nil {
				expectedScheme, expectedHost = expected.Scheme, expected.Host
			}
		}
		if !strings.EqualFold(got.Scheme, expectedScheme) || !strings.EqualFold(got.Host, expectedHost) {
			return echo.NewHTTPError(http.StatusForbidden, "cross-origin request denied")
		}
		return next(c)
	}
}
