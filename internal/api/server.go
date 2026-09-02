package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/riverqueue/river"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/notification"
	"github.com/vancanhuit/simplebank/internal/token"
)

// headerXForwardedHost carries the original Host a reverse proxy received, so
// the app can log the client-facing hostname rather than its own upstream
// address. Echo has no constant for it (unlike X-Forwarded-For/-Proto).
const headerXForwardedHost = "X-Forwarded-Host"

type Server struct {
	config                 config.Config
	store                  store.Store
	tokenMaker             *token.JWTMaker
	riverClient            *river.Client[pgx.Tx]
	subscribeNotifications func(string) (<-chan uuid.UUID, func())
	notificationKeepalive  time.Duration
	readiness              func(context.Context) error
	router                 *echo.Echo
}

func NewServer(
	cfg config.Config,
	st store.Store,
	maker *token.JWTMaker,
	riverClient *river.Client[pgx.Tx],
	notificationHub *notification.Hub,
	readiness func(context.Context) error,
) (*Server, error) {
	if notificationHub == nil {
		notificationHub = notification.NewHub()
	}
	if readiness == nil {
		readiness = func(context.Context) error { return nil }
	}
	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler,
		Validator:        newValidator(),
	})

	extractor, err := clientIPExtractor(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	e.IPExtractor = extractor

	e.Use(middleware.Recover())
	e.Use(trustedForwardingHeaders(cfg.TrustedProxies))
	e.Use(middleware.RequestID())
	e.Use(requestLogger())
	e.Use(securityHeaders())
	e.Use(middleware.BodyLimit(1 << 20))
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: 30 * time.Second,
		Skipper: func(c *echo.Context) bool {
			return c.Request().URL.Path == "/api/v1/notifications/stream"
		},
	}))

	s := &Server{
		config:                 cfg,
		store:                  st,
		tokenMaker:             maker,
		riverClient:            riverClient,
		subscribeNotifications: notificationHub.Subscribe,
		notificationKeepalive:  15 * time.Second,
		readiness:              readiness,
		router:                 e,
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.router }

// clientIPExtractor builds the RealIP() strategy. It reads the client address
// from the X-Forwarded-For header, but only trusts the hop closest to us when it
// falls inside a trusted proxy range — a directly-connected public client's
// spoofed XFF header is ignored. Forwarding headers are ignored unless trusted
// proxy ranges are explicitly configured.
func clientIPExtractor(trustedProxies []string) (echo.IPExtractor, error) {
	if len(trustedProxies) == 0 {
		return echo.ExtractIPDirect(), nil
	}
	opts := []echo.TrustOption{
		echo.TrustLoopback(false),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	}
	for _, cidr := range trustedProxies {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parsing trusted proxy CIDR %q: %w", cidr, err)
		}
		opts = append(opts, echo.TrustIPRange(ipNet))
	}
	return echo.ExtractIPFromXFFHeader(opts...), nil
}

func trustedForwardingHeaders(trustedProxies []string) echo.MiddlewareFunc {
	networks := make([]*net.IPNet, 0, len(trustedProxies))
	for _, cidr := range trustedProxies {
		_, network, _ := net.ParseCIDR(cidr) // validated by clientIPExtractor
		networks = append(networks, network)
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			host, _, _ := net.SplitHostPort(c.Request().RemoteAddr)
			remoteIP := net.ParseIP(host)
			trusted := false
			for _, network := range networks {
				if network.Contains(remoteIP) {
					trusted = true
					break
				}
			}
			if !trusted {
				header := c.Request().Header
				header.Del(echo.HeaderXForwardedFor)
				header.Del(echo.HeaderXForwardedProto)
				header.Del(headerXForwardedHost)
			}
			return next(c)
		}
	}
}

func securityHeaders() echo.MiddlewareFunc {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; " +
		"img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
		"frame-ancestors 'none'; form-action 'self'"
	return middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "0",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: true,
		ContentSecurityPolicy: csp,
		ReferrerPolicy:        "no-referrer",
	})
}

// forwardedHost returns the original client-facing host. Behind a proxy that
// rewrites the Host header, the X-Forwarded-Host value carries the hostname the
// client actually requested; its first entry is the outermost host.
func forwardedHost(host string, header http.Header) string {
	fwd := header.Get(headerXForwardedHost)
	if fwd == "" {
		return host
	}
	if i := strings.IndexByte(fwd, ','); i >= 0 {
		fwd = fwd[:i]
	}
	return strings.TrimSpace(fwd)
}

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
				slog.String("scheme", c.Scheme()),
				slog.String("host", forwardedHost(v.Host, c.Request().Header)),
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
