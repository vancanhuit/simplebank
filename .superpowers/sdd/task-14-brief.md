# Task 14: Migration runner (goose session locker) + wire subcommands (serve, worker)

**Files:**
- Create: `internal/db/migrate.go`
- Modify: `cmd/app/main.go`

## Produces
- `func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error` (package `store`) — runs domain goose migrations under a Postgres session-level advisory lock.
- CLI with two subcommands: `simplebank serve`, `simplebank worker`. Both run domain + River migrations on startup. NO `migrate` subcommand.

## Step 1: `internal/db/migrate.go`
```go
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/vancanhuit/simplebank/internal/db/migrations"
)

// MigrateSchema applies pending domain migrations under a PostgreSQL
// session-level advisory lock, so concurrent replicas starting together
// serialize migration application safely.
func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDB := stdlib.OpenDBFromPool(pool)

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		sqlDB,
		migrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return err
	}

	_, err = provider.Up(ctx)
	return err
}
```
COMPATIBILITY — verify against installed goose v3: `goose.NewProvider(dialect goose.Dialect, db *sql.DB, fsys fs.FS, ...goose.ProviderOption) (*goose.Provider, error)`, `goose.DialectPostgres`, `goose.WithSessionLocker(lock.SessionLocker)`, `lock.NewPostgresSessionLocker() (lock.SessionLocker, error)` in package `github.com/pressly/goose/v3/lock`, and `provider.Up(ctx) ([]*goose.MigrationResult, error)`. `stdlib.OpenDBFromPool(pool)` is confirmed present (pgx v5.10.0). If any identifier differs, adjust to the installed names but keep behavior: apply all Up migrations from `migrations.FS` under a session-level advisory lock. NOTE: goose Provider requires session locking to be enabled via the locker option (which this does); do not also call the legacy `goose.Up` global function.

## Step 2: Rewrite `cmd/app/main.go`
```go
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
```
COMPATIBILITY — verify: `pool.Ping(ctx)` (pgxpool), `echo.StartConfig{Address, GracefulTimeout}.Start(ctx, handler)` (Echo v5 — used in current main.go), River `riverClient.Start(ctx)` / `riverClient.Stop(ctx)` (v0.40.0), urfave/cli v3 `cmd.Run(ctx, os.Args)` and `Action func(context.Context, *cli.Command) error`. Adjust to installed identifiers if needed.

IMPORTANT: `/readyz` must be registered exactly once. Task 13's `routes.go` registers `/livez` only (NOT `/readyz`). Here we register `/readyz` on the returned handler. Confirm there is no other `/readyz` registration; if `registerRoutes` accidentally registers `/readyz`, remove it there.

## Step 3: Verify build + unit tests + lint
- `go build ./...` and `go vet ./...` → clean.
- `go test -race ./...` → all unit tests PASS (integration tests are tag-gated and won't run without the tag).
- `mise run golangci-lint` → clean (fix any lint issues introduced).

## Step 4: Commit
```bash
git add cmd/app/main.go internal/db/migrate.go
git commit -m "feat: wire serve and worker subcommands with startup migrations"
```

## Global Constraints
- Migrations run on startup for both subcommands, domain under a goose Postgres session locker.
- Graceful shutdown via `signal.NotifyContext`; `serve` uses Echo StartConfig graceful timeout; `worker` drains via `riverClient.Stop`.
- `livez` no DB dependency; `readyz` does a short-deadline DB ping.
- slog JSON to stdout. Never log secrets.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-14-report.md`, noting the goose Provider API used and any adjustments, and confirming `/readyz` is registered once. Return only: status, commit hash(es), one-line build/test/lint summary, concerns.
