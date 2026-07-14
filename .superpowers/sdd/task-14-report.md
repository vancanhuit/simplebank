# Task 14 Report: Migration runner (goose session locker) + wire subcommands (serve, worker)

**Status:** DONE
**Commit:** `434d12d10f5996273c0a27d827fe079b7e6f8dc1` — `feat: wire serve and worker subcommands with startup migrations`

## Files
- Created: `internal/db/migrate.go` (`func MigrateSchema(ctx, pool)` in package `store`).
- Replaced: `cmd/app/main.go` (minimal Echo starter → urfave/cli v3 CLI with `serve` and `worker` subcommands).

## goose Provider API (verified against installed goose v3.27.2)
All identifiers in the brief matched the installed version exactly — **no adjustments required**:
- `goose.NewProvider(dialect Dialect, db *sql.DB, fsys fs.FS, ...ProviderOption) (*Provider, error)` — `provider.go:58`.
- `goose.DialectPostgres` — `dialect.go:18`.
- `goose.WithSessionLocker(locker lock.SessionLocker) ProviderOption` — `provider_options.go:74`.
- `lock.NewPostgresSessionLocker(...SessionLockerOption) (SessionLocker, error)` in `github.com/pressly/goose/v3/lock` — `lock/postgres.go:71`.
- `(*Provider).Up(ctx context.Context) ([]*MigrationResult, error)` — `provider.go:244`.
- `stdlib.OpenDBFromPool(pool)` confirmed present (pgx v5.10.0).

Behavior: applies all Up migrations from `migrations.FS` under a Postgres session-level advisory lock. The legacy global `goose.Up` is NOT called; migration is driven solely through the Provider with the session locker enabled.

## `/readyz` registration — confirmed registered exactly once
- Grep for `readyz` across `internal/**/*.go` returned **empty** — `registerRoutes` / `routes.go` registers only `/livez` (no `/readyz`).
- `/readyz` is registered a single time in `runServe` on the handler returned by `server.Handler()`, performing a 2s-deadline `pool.Ping`.

## Signature compatibility (verified against call sites)
- `config.Flags()`, `config.FromCommand(cmd)`, `(config.Config).Validate()` — present.
- `store.NewStore(pool)`, `store.MigrateSchema(ctx, pool)` — present.
- `worker.Migrate(ctx, pool)`, `worker.NewClient(ctx, pool, maxWorkers, st, mailer, baseURL) (*river.Client[pgx.Tx], error)` — match.
- `api.NewServer(cfg, st, maker, riverClient)` + `(*Server).Handler() *echo.Echo` — match.
- `token.NewJWTMaker`, `mail.NewSMTPMailer(cfg)` — present.
- Echo v5 `echo.StartConfig{Address, GracefulTimeout}.Start(ctx, e)`, `*echo.Context` handlers — match.
- River v0.40.0 `riverClient.Start(ctx)` / `riverClient.Stop(ctx)` — match.
- urfave/cli v3.10.1 `cmd.Run(ctx, os.Args)`, `Action func(context.Context, *cli.Command) error` — match.

## Verification (all clean)
- `go build ./...` → BUILD_OK
- `go vet ./...` → VET_OK
- `go test -race ./...` → all unit test packages PASS (api, config, db, token, util); integration is tag-gated and not run.
- `mise run golangci-lint` → **0 issues** (golangci-lint 2.12.2).
- `go mod tidy` was NOT run, per instructions.

## Constraints satisfied
- Both `serve` and `worker` run domain (goose session locker) + River migrations on startup via `runMigrations`.
- Graceful shutdown: `signal.NotifyContext` cancels ctx; `serve` uses Echo `StartConfig.GracefulTimeout` (10s) and ignores `http.ErrServerClosed`; `worker` drains via `riverClient.Stop` with a 10s shutdown context.
- `livez` has no DB dependency; `readyz` does a 2s-deadline DB ping.
- slog JSON handler to stdout; no secrets logged.
- No `migrate` subcommand added.

## Concerns
None.

## Fix pass: graceful worker drain + close migration sql.DB

**Status:** DONE

Two fixes applied:
1. `internal/db/migrate.go`: added `defer sqlDB.Close()` after `stdlib.OpenDBFromPool(pool)` to release the database/sql layer wrapper (does not close the underlying pgxpool).
2. `cmd/app/main.go` `runWorker`: changed `riverClient.Start(ctx)` to `riverClient.Start(context.Background())` so River job fetching runs under a non-cancellable context; graceful drain is driven solely by `<-ctx.Done()` + `riverClient.Stop(shutdownCtx)` (fresh 10s timeout).

**River v0.40.0 semantics confirmed (via `go doc`):**
- `Start(ctx)`: cancelling Start's ctx triggers an abrupt/hard shutdown that also cancels contexts of currently-running jobs (escalates to hard stop).
- `Stop(ctx)`: performs a graceful shutdown — signals producers to stop fetching new jobs and waits for fetched/in-progress jobs to complete; returns early if the passed ctx is done.
- Therefore `Start(context.Background())` + `Stop(ctx)` is the documented graceful-drain pattern: Start's ctx controls fetching (kept non-cancellable to avoid racing an abrupt shutdown against the explicit Stop), and Stop waits for running jobs to finish.

**Verification:** `go build ./...` PASS, `go vet ./...` PASS, `go test -race ./...` PASS, `mise run golangci-lint` PASS (0 issues).
