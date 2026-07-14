package api

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)

	v1 := s.router.Group("/api/v1")

	v1.POST("/users", s.createUser)
	v1.POST("/users/login", s.loginUser)
	v1.POST("/tokens/renew", s.renewToken)
	v1.GET("/users/verify_email", s.verifyEmail)

	auth := v1.Group("")
	auth.Use(s.authMiddleware())
	auth.POST("/accounts", s.createAccount)
	auth.GET("/accounts/:id", s.getAccount)
	auth.GET("/accounts", s.listAccounts)
	auth.POST("/transfers", s.createTransfer)
}
