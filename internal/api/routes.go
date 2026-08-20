package api

import (
	"github.com/labstack/echo/v5/middleware"
)

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)
	s.router.GET("/readyz", s.readyz)

	v1 := s.router.Group("/api/v1")

	authLimiter := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(5))

	v1.POST("/users", s.createUser, authLimiter)
	v1.POST("/users/login", s.loginUser)
	v1.POST("/users/logout", s.logoutUser, authLimiter)
	v1.POST("/tokens/renew", s.renewToken, authLimiter)
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
}
