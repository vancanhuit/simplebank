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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
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

func runServe(ctx context.Context, cmd *cli.Command) error {
	cfg, err := mustConfig(cmd)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DBSource)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		return err
	}

	st := store.NewStore(pool)
	maker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		return err
	}
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		return err
	}
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, "http://localhost"+cfg.HTTPAddr)
	if err != nil {
		return err
	}

	server, err := api.NewServer(cfg, st, maker, riverClient)
	if err != nil {
		return err
	}

	e := server.Handler()
	e.GET("/readyz", func(c *echo.Context) error {
		pingCtx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	sc := echo.StartConfig{Address: cfg.HTTPAddr, GracefulTimeout: 10 * time.Second}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runWorker(ctx context.Context, cmd *cli.Command) error {
	cfg, err := mustConfig(cmd)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DBSource)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		return err
	}

	st := store.NewStore(pool)
	mailer, err := mail.NewSMTPMailer(cfg)
	if err != nil {
		return err
	}
	riverClient, err := worker.NewClient(ctx, pool, cfg.RiverMaxWorkers, st, mailer, "http://localhost"+cfg.HTTPAddr)
	if err != nil {
		return err
	}

	if err := riverClient.Start(ctx); err != nil {
		return err
	}
	slog.Info("worker started")

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return riverClient.Stop(shutdownCtx)
}
