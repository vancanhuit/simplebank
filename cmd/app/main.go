package main

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/urfave/cli/v3"

	"github.com/vancanhuit/simplebank/frontend"
	"github.com/vancanhuit/simplebank/internal/api"
	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/mail"
	"github.com/vancanhuit/simplebank/internal/notification"
	"github.com/vancanhuit/simplebank/internal/token"
	"github.com/vancanhuit/simplebank/internal/worker"
)

// Injected at build time via -ldflags -X (see the app:build mise task).
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cmd := newCommand()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		logger.Error("command failed", "error", err)
		return 1
	}
	return 0
}

func newCommand() *cli.Command {
	return &cli.Command{
		Name:    "simplebank",
		Usage:   "SimpleBank cloud-native service",
		Version: version,
		Flags:   config.Flags(),
		Commands: []*cli.Command{
			{Name: "serve", Usage: "Run the HTTP API server and background worker", Action: runServe},
			{Name: "healthcheck", Usage: "Probe the local liveness endpoint (for container HEALTHCHECK)", Action: runHealthcheck},
			{
				Name:  "version",
				Usage: "Print version information",
				Action: func(_ context.Context, _ *cli.Command) error {
					fmt.Printf("version:    %s\ncommit:     %s\nbuild date: %s\n", version, commit, buildDate)
					return nil
				},
			},
		},
	}
}

func mustConfig(cmd *cli.Command) (config.Config, error) {
	cfg := config.FromCommand(cmd)
	return cfg, cfg.Validate()
}

// newPool builds the pgx connection pool. Sizing (max/min conns) is left to the
// operator via config or the DSN, since it depends on the database's connection
// limit and replica count. A lifetime jitter spreads connection expiry so a
// batch opened at startup does not all recycle at the same instant.
func newPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DBSource)
	if err != nil {
		return nil, err
	}
	if cfg.DBMaxConns > 0 {
		poolCfg.MaxConns = cfg.DBMaxConns
	}
	if cfg.DBMinConns > 0 {
		poolCfg.MinConns = cfg.DBMinConns
	}
	if cfg.DBMaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.DBMaxConnLifetime
	}
	if cfg.DBMaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	}
	poolCfg.MaxConnLifetimeJitter = poolCfg.MaxConnLifetime / 10
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if err := store.MigrateSchema(ctx, pool); err != nil {
		return err
	}
	slog.Info("domain migrations applied")
	if err := worker.Migrate(ctx, pool); err != nil {
		return err
	}
	slog.Info("river migrations applied")
	return nil
}

// appDeps holds the service dependencies. The caller owns closing the pool.
type appDeps struct {
	cfg                  config.Config
	pool                 *pgxpool.Pool
	store                store.Store
	mailer               mail.Mailer
	riverClient          *river.Client[pgx.Tx]
	notificationHub      *notification.Hub
	notificationListener *notification.Listener
}

// buildApp assembles dependencies in the required order: open the pool, run
// migrations, then construct the store and long-lived services. On any error
// the pool is closed before returning so the caller does not leak it.
func buildApp(ctx context.Context, cmd *cli.Command) (*appDeps, error) {
	cfg, err := mustConfig(cmd)
	if err != nil {
		return nil, err
	}
	pool, err := newPool(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// Close the pool on any failure after this point; cleared on success so the
	// caller owns the open pool.
	defer func() {
		if pool != nil {
			pool.Close()
		}
	}()

	if err := runMigrations(ctx, pool); err != nil {
		return nil, err
	}
	st := store.New(pool)
	hub := notification.NewHub()
	listener := notification.NewListener(pool.Config().ConnConfig, hub)
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.PublicBaseURL, "http://localhost"+cfg.HTTPAddr)
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, baseURL)
	if err != nil {
		return nil, err
	}

	deps := &appDeps{
		cfg:                  cfg,
		pool:                 pool,
		store:                st,
		mailer:               mailer,
		riverClient:          riverClient,
		notificationHub:      hub,
		notificationListener: listener,
	}
	pool = nil // success: hand the open pool to the caller
	return deps, nil
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	app, err := buildApp(ctx, cmd)
	if err != nil {
		return err
	}
	defer func() {
		app.pool.Close()
		slog.Info("database pool closed")
	}()

	maker, err := token.NewJWTMaker(app.cfg.JWTSecret)
	if err != nil {
		return err
	}

	server, err := api.NewServer(
		app.cfg,
		app.store,
		maker,
		app.riverClient,
		app.notificationHub,
		app.pool.Ping,
	)
	if err != nil {
		return err
	}

	dist, err := frontend.Dist()
	if err != nil {
		return err
	}
	server.RegisterSPA(dist)

	return runServices(ctx, app.notificationListener, app.riverClient, func(ctx context.Context) error {
		return startServer(ctx, app.cfg, server.Handler())
	})
}

type serviceLifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
}

func runServices(
	ctx context.Context,
	listener serviceLifecycle,
	worker serviceLifecycle,
	serve func(context.Context) error,
) error {
	lifecycleCtx := context.WithoutCancel(ctx)
	if err := listener.Start(ctx); err != nil {
		return fmt.Errorf("starting notification listener: %w", err)
	}
	slog.Info("notification listener started")

	if err := worker.Start(ctx); err != nil {
		shutdownCtx, cancel := context.WithTimeout(lifecycleCtx, 10*time.Second)
		listenerErr := listener.Stop(shutdownCtx)
		cancel()
		return errors.Join(fmt.Errorf("starting worker: %w", err), listenerErr)
	}
	slog.Info("worker started")

	serverErr := serve(ctx)
	slog.Info("http server shut down", "cause", context.Cause(ctx))
	slog.Info("worker shutting down", "cause", context.Cause(ctx))
	workerShutdownCtx, cancelWorkerShutdown := context.WithTimeout(lifecycleCtx, 10*time.Second)
	workerErr := worker.Stop(workerShutdownCtx)
	cancelWorkerShutdown()

	slog.Info("notification listener shutting down", "cause", context.Cause(ctx))
	listenerShutdownCtx, cancelListenerShutdown := context.WithTimeout(lifecycleCtx, 10*time.Second)
	listenerErr := listener.Stop(listenerShutdownCtx)
	cancelListenerShutdown()

	return errors.Join(serverErr, workerErr, listenerErr)
}

// startServer runs the HTTP server with hardened timeouts and graceful
// shutdown, serving over TLS when a certificate is configured and plain HTTP
// otherwise.
func startServer(ctx context.Context, cfg config.Config, handler http.Handler) error {
	sc := echo.StartConfig{
		Address:         cfg.HTTPAddr,
		GracefulTimeout: 10 * time.Second,
		BeforeServeFunc: func(s *http.Server) error { configureServer(s); return nil },
	}

	if cfg.TLSCertFile != "" {
		cert, err := os.ReadFile(cfg.TLSCertFile)
		if err != nil {
			return fmt.Errorf("reading tls cert file %s: %w", cfg.TLSCertFile, err)
		}
		key, err := os.ReadFile(cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("reading tls key file %s: %w", cfg.TLSKeyFile, err)
		}
		slog.Info("starting server with TLS", "addr", cfg.HTTPAddr)
		if err := sc.StartTLS(ctx, handler, cert, key); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}

	if err := sc.Start(ctx, handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func configureServer(s *http.Server) {
	s.ReadHeaderTimeout = 5 * time.Second // slow-loris protection
	s.ReadTimeout = 30 * time.Second      // headers and request body
	s.WriteTimeout = 30 * time.Second     // response write
	s.IdleTimeout = 120 * time.Second     // keep-alive connections
}

// runHealthcheck probes the local /livez endpoint so a container HEALTHCHECK can
// call the binary itself (the distroless image has no shell or curl). It reads
// only the address and TLS flags, so it works without a database or full config.
func runHealthcheck(ctx context.Context, cmd *cli.Command) error {
	addr := cmp.Or(cmd.String("http-addr"), ":8080")
	certFile := cmd.String("tls-cert-file")

	scheme := "http"
	transport := &http.Transport{}
	if certFile != "" {
		scheme = "https"
		// Loopback self-probe against the app's own self-signed dev cert; the CA
		// is not mounted in the container, so skip verification. This is a
		// liveness check to localhost, not a trust decision on remote traffic.
		//nolint:gosec // The healthcheck only connects to this process on localhost.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s://localhost%s/livez", scheme, addr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck failed: status %d", resp.StatusCode)
	}
	return nil
}
