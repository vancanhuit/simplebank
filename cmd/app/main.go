package main

import (
	"cmp"
	"context"
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

	"github.com/vancanhuit/simplebank/internal/api"
	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/mail"
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cmd := &cli.Command{
		Name:    "simplebank",
		Usage:   "SimpleBank cloud-native service",
		Version: version,
		Flags:   config.Flags(),
		Commands: []*cli.Command{
			{Name: "serve", Usage: "Run the HTTP API server", Action: runServe},
			{Name: "worker", Usage: "Run the background worker", Action: runWorker},
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
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
		poolCfg.MaxConns = int32(cfg.DBMaxConns)
	}
	if cfg.DBMinConns > 0 {
		poolCfg.MinConns = int32(cfg.DBMinConns)
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

// appDeps holds the shared dependencies both entrypoints need. The caller owns
// closing the pool.
type appDeps struct {
	cfg         config.Config
	pool        *pgxpool.Pool
	store       store.Store
	mailer      mail.Mailer
	riverClient *river.Client[pgx.Tx]
}

// buildApp assembles dependencies in the required order: open the pool, run
// migrations, then construct the store, mailer, and river client. On any error
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
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		return nil, err
	}
	baseURL := cmp.Or(cfg.PublicBaseURL, "http://localhost"+cfg.HTTPAddr)
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, baseURL)
	if err != nil {
		return nil, err
	}

	deps := &appDeps{cfg: cfg, pool: pool, store: st, mailer: mailer, riverClient: riverClient}
	pool = nil // success: hand the open pool to the caller
	return deps, nil
}

func runServe(ctx context.Context, cmd *cli.Command) error {
	app, err := buildApp(ctx, cmd)
	if err != nil {
		return err
	}
	defer app.pool.Close()

	maker, err := token.NewJWTMaker(app.cfg.JWTSecret)
	if err != nil {
		return err
	}

	server, err := api.NewServer(app.cfg, app.store, maker, app.riverClient, app.pool.Ping)
	if err != nil {
		return err
	}

	sc := echo.StartConfig{
		Address:         app.cfg.HTTPAddr,
		GracefulTimeout: 10 * time.Second,
		BeforeServeFunc: func(s *http.Server) error {
			s.ReadHeaderTimeout = 5 * time.Second // slow-loris protection
			s.WriteTimeout = 30 * time.Second     // response write
			s.IdleTimeout = 120 * time.Second     // keep-alive connections
			return nil
		},
	}

	if app.cfg.TLSCertFile != "" {
		cert, err := os.ReadFile(app.cfg.TLSCertFile)
		if err != nil {
			return fmt.Errorf("reading tls cert file %s: %w", app.cfg.TLSCertFile, err)
		}
		key, err := os.ReadFile(app.cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("reading tls key file %s: %w", app.cfg.TLSKeyFile, err)
		}
		slog.Info("starting server with TLS", "addr", app.cfg.HTTPAddr)
		if err := sc.StartTLS(ctx, server.Handler(), cert, key); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}

	if err := sc.Start(ctx, server.Handler()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runWorker(ctx context.Context, cmd *cli.Command) error {
	app, err := buildApp(ctx, cmd)
	if err != nil {
		return err
	}
	defer app.pool.Close()

	if err := app.riverClient.Start(context.Background()); err != nil {
		return err
	}
	slog.Info("worker started")

	<-ctx.Done()
	slog.Info("worker shutting down", "cause", context.Cause(ctx))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return app.riverClient.Stop(shutdownCtx)
}
