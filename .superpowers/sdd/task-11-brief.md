# Task 11: River job args, worker, and client (+ crypto-secure secret code)

**Files:**
- Create: `internal/util/secure.go`
- Test: `internal/util/secure_test.go`
- Create: `internal/worker/verify_email.go`
- Create: `internal/worker/client.go`

## SECURITY REQUIREMENT (applies here)
The email verification `secret_code` must be generated with a CRYPTO-SECURE random
source (`crypto/rand`), NOT `util.RandomString` (which uses `math/rand`). Add a
`util.SecureToken` helper and use it in the worker.

## Step 1: Write `internal/util/secure_test.go` (failing)
```go
package util

import "testing"

func TestSecureToken(t *testing.T) {
	a, err := SecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || b == "" {
		t.Fatal("token should not be empty")
	}
	if a == b {
		t.Fatal("two secure tokens should differ")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}
```
Run `go test ./internal/util/ -run TestSecureToken -v` → FAIL.

## Step 2: Write `internal/util/secure.go`
```go
package util

import (
	"crypto/rand"
	"encoding/hex"
)

// SecureToken returns a cryptographically secure random token encoded as a
// hex string. n is the number of random bytes; the returned string is 2*n chars.
func SecureToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```
Run the test → PASS.

## Step 3: Write `internal/worker/verify_email.go`
```go
package worker

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/mail"
	"github.com/vancanhuit/simplebank/internal/util"
)

type SendVerifyEmailArgs struct {
	Username string `json:"username"`
}

func (SendVerifyEmailArgs) Kind() string { return "send_verify_email" }

type SendVerifyEmailWorker struct {
	river.WorkerDefaults[SendVerifyEmailArgs]
	store   store.Store
	mailer  mail.Mailer
	baseURL string
}

func NewSendVerifyEmailWorker(st store.Store, mailer mail.Mailer, baseURL string) *SendVerifyEmailWorker {
	return &SendVerifyEmailWorker{store: st, mailer: mailer, baseURL: baseURL}
}

func (w *SendVerifyEmailWorker) Work(ctx context.Context, job *river.Job[SendVerifyEmailArgs]) error {
	user, err := w.store.GetUser(ctx, job.Args.Username)
	if err != nil {
		return err
	}

	code, err := util.SecureToken(32)
	if err != nil {
		return err
	}

	ve, err := w.store.CreateVerifyEmail(ctx, sqlcdb.CreateVerifyEmailParams{
		Username:   user.Username,
		Email:      user.Email,
		SecretCode: code,
	})
	if err != nil {
		return err
	}

	link := fmt.Sprintf("%s/api/v1/users/verify_email?id=%s&code=%s",
		w.baseURL, ve.ID.String(), ve.SecretCode)
	body := fmt.Sprintf(
		`Hello %s,<br/>Please <a href="%s">click here</a> to verify your email address.`,
		user.FullName, link)

	return w.mailer.Send(ctx, user.Email, "Welcome to SimpleBank", body)
}
```

## Step 4: Write `internal/worker/client.go`
```go
package worker

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/mail"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func NewClient(
	ctx context.Context,
	pool *pgxpool.Pool,
	maxWorkers int,
	st store.Store,
	mailer mail.Mailer,
	baseURL string,
) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewSendVerifyEmailWorker(st, mailer, baseURL))

	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		},
		Workers: workers,
	})
}
```

COMPATIBILITY — verify River's API against the installed version (read `go doc github.com/riverqueue/river` etc.) and adjust to the exact identifiers so it compiles:
- `river.NewClient(driver, *river.Config)` returning `(*river.Client[pgx.Tx], error)`.
- `river.Config{ Queues map[string]river.QueueConfig, Workers *river.Workers }`; `river.QueueDefault`; `river.QueueConfig{MaxWorkers: int}`.
- `river.NewWorkers()`, `river.AddWorker(workers, worker)`.
- `river.WorkerDefaults[Args]`, `river.Job[Args]`, worker `Work(ctx, *river.Job[Args]) error`, args `Kind() string`.
- `riverpgxv5.New(pool)`.
- `rivermigrate.New(driver, *rivermigrate.Config)` (second arg may be `nil`), `migrator.Migrate(ctx, rivermigrate.DirectionUp, *rivermigrate.MigrateOpts)` (opts may be `nil`).
If any differ, use the installed identifiers; keep behavior (a client with a default queue running `SendVerifyEmailWorker`, and a `Migrate` that applies River's schema). The `ctx` param of `NewClient` may be unused — if the installed API doesn't need it, keep the param (callers pass it) but you may `_ = ctx` or drop it and update callers is NOT allowed (Task 14 calls `NewClient(ctx, pool, ...)`), so keep the signature. If Go complains ctx is unused, name it `_ context.Context`? No — keep `ctx context.Context` and reference it or accept the unused-param (unused function params are allowed in Go; only unused locals error). So this is fine.

## Step 5: Verify build
`go build ./internal/worker/`, `go build ./...`, `go vet ./...` → clean.
`go test ./internal/util/ -v` → PASS (incl. SecureToken).

## Step 6: Commit
```bash
git add internal/util/secure.go internal/util/secure_test.go internal/worker/
git commit -m "feat: add River verify-email worker with crypto-secure secret code"
```

## Global Constraints
- `secret_code` MUST use crypto/rand via `util.SecureToken` (NOT math/rand).
- Never log the secret code or email body.
- Worker is idempotent-friendly; River retries on failure.
- Confirmed generated names (Task 5): `CreateVerifyEmailParams{Username, Email, SecretCode}`, `VerifyEmail.ID`, `VerifyEmail.SecretCode`.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-11-report.md`, listing the exact River identifiers used and the River version. Return only: status, commit hash(es), one-line test/build summary, River version + key identifiers, concerns.
