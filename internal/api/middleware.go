package api

import (
	"errors"
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v5"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

const (
	authContextKey = "user"
	roleDepositor  = "depositor"
)

func (s *Server) authMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey: []byte(s.config.JWTSecret),
		ContextKey: authContextKey,
		NewClaimsFunc: func(c *echo.Context) jwt.Claims {
			return token.NewExpectedPayload(token.Access)
		},
		SuccessHandler: func(c *echo.Context) error {
			payload, err := authPayload(c)
			if err != nil {
				return err
			}
			err = s.store.ValidateAccessSession(
				c.Request().Context(),
				payload.ID,
				payload.Username,
				time.Now(),
			)
			if errors.Is(err, store.ErrInvalidSession) {
				return echo.ErrUnauthorized
			}
			return err
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
