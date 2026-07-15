package main

import (
	"context"
	"errors"
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cmd := &cli.Command{
		Name:  "simplebank",
		Usage: "SimpleBank cloud-native service",
		Flags: config.Flags(),
		Commands: []*cli.Command{
			{Name: "serve", Usage: "Run the HTTP API server", Action: runServe},
			{Name: "worker", Usage: "Run the background worker", Action: runWorker},
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
	pool, err := pgxpool.New(ctx, cfg.DBSource)
	if err != nil {
		return nil, err
	}
	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	st := store.NewStore(pool)
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		pool.Close()
		return nil, err
	}
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, "http://localhost"+cfg.HTTPAddr)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &appDeps{cfg: cfg, pool: pool, store: st, mailer: mailer, riverClient: riverClient}, nil
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

	sc := echo.StartConfig{Address: app.cfg.HTTPAddr, GracefulTimeout: 10 * time.Second}
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return app.riverClient.Stop(shutdownCtx)
}
