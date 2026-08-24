package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)
	s.router.GET("/readyz", s.readyz)

	v1 := s.router.Group("/api/v1")
	v1.Use(noStore)

	endpointClient := func(c *echo.Context) (string, error) {
		return c.Request().Method + " " + c.Request().URL.Path + "\x00" + c.RealIP(), nil
	}
	credentialLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store:               middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{Rate: 0.2, Burst: 5, ExpiresIn: 15 * time.Minute}),
		IdentifierExtractor: endpointClient,
		DenyHandler: func(_ *echo.Context, _ string, _ error) error {
			return newAPIError(http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
		},
	})
	authLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store:               middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{Rate: 1, Burst: 5, ExpiresIn: 15 * time.Minute}),
		IdentifierExtractor: endpointClient,
		DenyHandler: func(_ *echo.Context, _ string, _ error) error {
			return newAPIError(http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
		},
	})

	v1.POST("/users", s.createUser, credentialLimiter)
	v1.POST("/users/login", s.loginUser, credentialLimiter)
	v1.POST("/users/logout", s.logoutUser, s.sameOrigin, authLimiter)
	v1.POST("/tokens/renew", s.renewToken, s.sameOrigin, authLimiter)
	v1.POST("/users/verify_email/resend", s.resendVerifyEmail, authLimiter)
	v1.GET("/users/verify_email", s.verifyEmail, authLimiter)
	v1.GET("/transfer-limits", s.transferLimits)
	v1.GET("/account-opening-limits", s.accountOpeningLimits)

	auth := v1.Group("")
	auth.Use(s.authMiddleware())
	auth.POST("/accounts", s.createAccount)
	auth.GET("/accounts/:id", s.getAccount)
	auth.GET("/accounts", s.listAccounts)
	auth.GET("/accounts/:id/transfers", s.listTransfers)
	auth.POST("/transfers", s.createTransfer)
	auth.GET("/notifications/stream", s.streamNotifications)
	auth.GET("/notifications", s.listNotifications)
	auth.PUT("/notifications/:id/read", s.markNotificationRead)
	auth.PUT("/notifications/read-all", s.markAllNotificationsRead)
}

func noStore(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		return next(c)
	}
}
