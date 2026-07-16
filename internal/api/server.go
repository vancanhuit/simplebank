package api

import (
	"context"
	"log/slog"
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
	readiness   func(context.Context) error
	router      *echo.Echo
}

func NewServer(
	cfg config.Config,
	st store.Store,
	maker token.Maker,
	riverClient *river.Client[pgx.Tx],
	readiness func(context.Context) error,
) (*Server, error) {
	if readiness == nil {
		readiness = func(context.Context) error { return nil }
	}
	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler,
		Validator:        newValidator(),
	})

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(requestLogger())
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(1 << 20))
	e.Use(middleware.ContextTimeout(30 * time.Second))

	s := &Server{
		config:      cfg,
		store:       st,
		tokenMaker:  maker,
		riverClient: riverClient,
		readiness:   readiness,
		router:      e,
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) livez(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// readyz reports whether the service can serve traffic. The readiness probe is
// injected at construction so the Server owns the route while callers decide
// what "ready" means (typically a database ping).
func (s *Server) readyz(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	if err := s.readiness(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

// requestLogger returns a request logger middleware that logs the request path
// only (never the raw URI/query string) so that sensitive query parameters such
// as the verify-email `code` are never written to logs.
func requestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogLatency:       true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogMethod:        true,
		LogURIPath:       true,
		LogRequestID:     true,
		LogUserAgent:     true,
		LogStatus:        true,
		LogContentLength: true,
		LogResponseSize:  true,
		HandleError:      true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			logger := c.Logger()
			level := slog.LevelInfo
			msg := "REQUEST"
			if v.Error != nil {
				level = slog.LevelError
				msg = "REQUEST_ERROR"
			}
			attrs := []slog.Attr{
				slog.String("method", v.Method),
				slog.String("path", v.URIPath),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
				slog.String("host", v.Host),
				slog.String("bytes_in", v.ContentLength),
				slog.Int64("bytes_out", v.ResponseSize),
				slog.String("user_agent", v.UserAgent),
				slog.String("remote_ip", v.RemoteIP),
				slog.String("request_id", v.RequestID),
			}
			if v.Error != nil {
				attrs = append(attrs, slog.String("error", v.Error.Error()))
			}
			logger.LogAttrs(c.Request().Context(), level, msg, attrs...)
			return nil
		},
	})
}
