# Task 11 Report: River verify-email worker + crypto-secure secret code

**Status:** DONE

## Files created
- `internal/util/secure.go` — `SecureToken(n int) (string, error)` using `crypto/rand` + `encoding/hex`.
- `internal/util/secure_test.go` — `TestSecureToken` (TDD: written failing first, then passing).
- `internal/worker/verify_email.go` — `SendVerifyEmailArgs`, `SendVerifyEmailWorker`, `Work`.
- `internal/worker/client.go` — `Migrate`, `NewClient`.

## TDD sequence
1. Wrote `secure_test.go` → `go test ./internal/util/ -run TestSecureToken -v` = FAIL (undefined: SecureToken).
2. Wrote `secure.go` → test PASS.
3. Added worker files.
4. `go build ./...`, `go vet ./...` clean; `go test ./internal/util/ -v` all PASS.

## Security
- `secret_code` generated via `util.SecureToken(32)` (crypto/rand, 64 hex chars). NOT `util.RandomString` (math/rand).
- Secret code and email body are never logged.

## River version
`github.com/riverqueue/river v0.40.0` (and riverdriver/riverpgxv5, rivermigrate @ v0.40.0).

## River identifiers verified (via `go doc`) — all matched template exactly, no adjustments needed
- `river.NewClient[TTx any](driver riverdriver.Driver[TTx], config *river.Config) (*river.Client[TTx], error)`
- `river.Config{ Queues map[string]river.QueueConfig; Workers *river.Workers }`
- `river.QueueConfig{ MaxWorkers int }`
- `river.QueueDefault` (const)
- `river.NewWorkers() *river.Workers`
- `river.AddWorker[T river.JobArgs](workers *river.Workers, worker river.Worker[T])`
- `river.WorkerDefaults[T]`, `river.Job[T]`, worker `Work(ctx, *river.Job[Args]) error`, args `Kind() string`
- `riverpgxv5.New(pool) riverdriver.Driver[pgx.Tx]`
- `rivermigrate.New[TTx any](driver, config *rivermigrate.Config) (*Migrator[TTx], error)` (config may be nil)
- `(*rivermigrate.Migrator[TTx]).Migrate(ctx, direction rivermigrate.Direction, opts *rivermigrate.MigrateOpts) (*MigrateResult, error)` (opts may be nil)
- `rivermigrate.DirectionUp` (const)

## Signature preserved for Task 14
`NewClient(ctx context.Context, pool *pgxpool.Pool, maxWorkers int, st store.Store, mailer mail.Mailer, baseURL string) (*river.Client[pgx.Tx], error)`. The `ctx` param is currently unused by the River v0.40.0 API but retained (unused function params are legal in Go; callers in Task 14 pass it).

## Verification
- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./internal/util/ -v` — PASS (TestSecureToken + existing)

## Notes
- Did NOT run `go mod tidy` (per instruction; would strip deps needed by later tasks).

## Concerns
- None. `ctx` unused in `NewClient` is intentional per contract.

---

## Security fix (follow-up): HTML injection in verification email

**Status:** DONE
**Commit:** `cb88c21f3920c9742394866afe37b7efd06fb751`

- Finding: `internal/worker/verify_email.go` interpolated `user.FullName` (user-controlled) directly into the email HTML body via `fmt.Sprintf`, allowing HTML injection in mail clients.
- Fix: imported `"html"` and wrapped the full name with `html.EscapeString(user.FullName)` when building `body`. The link URL uses server-generated values (uuid + hex code) and was left unchanged.
- Build: `go build ./...` clean; `go vet ./...` clean.
