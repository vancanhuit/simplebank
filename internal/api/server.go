package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/riverqueue/river"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

type Server struct {
	config      config.Config
	store       store.Store
	tokenMaker  token.Maker
	riverClient *river.Client[pgx.Tx]
	router      *echo.Echo
}

func NewServer(
	cfg config.Config,
	st store.Store,
	maker token.Maker,
	riverClient *river.Client[pgx.Tx],
) (*Server, error) {
	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler,
		Validator:        newValidator(),
	})

	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(1 << 20))
	e.Use(middleware.ContextTimeout(30 * time.Second))
	e.Use(middleware.Recover())

	s := &Server{
		config:      cfg,
		store:       st,
		tokenMaker:  maker,
		riverClient: riverClient,
		router:      e,
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() *echo.Echo { return s.router }

func (s *Server) livez(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
