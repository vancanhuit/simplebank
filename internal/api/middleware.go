package api

import (
	jwt "github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v5"
	"github.com/labstack/echo/v5"

	"github.com/vancanhuit/simplebank/internal/token"
)

const authContextKey = "user"

func (s *Server) authMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey: []byte(s.config.JWTSecret),
		ContextKey: authContextKey,
		NewClaimsFunc: func(c *echo.Context) jwt.Claims {
			return new(token.Payload)
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
