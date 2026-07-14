# Task 2: Config package (urfave/cli flags + env value sources)

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

## Produces
- `type Config struct { HTTPAddr string; DBSource string; JWTSecret string; AccessTTL time.Duration; RefreshTTL time.Duration; SMTPHost string; SMTPPort int; SMTPUsername string; SMTPPassword string; SMTPFrom string; RiverMaxWorkers int }`
- `func (c Config) Validate() error` — error if `DBSource == ""`, `len(JWTSecret) < 32`, or `SMTPFrom == ""`.
- `func Flags() []cli.Flag` — urfave/cli v3 flags, each with `Sources: cli.EnvVars("...")`.
- `func FromCommand(cmd *cli.Command) Config`.

## Step 1: Write the failing test (`internal/config/config_test.go`)
```go
package config

import "testing"

func TestValidate(t *testing.T) {
	valid := Config{
		DBSource:  "postgres://u:p@localhost:5432/db",
		JWTSecret: "01234567890123456789012345678901",
		SMTPFrom:  "no-reply@example.com",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	tests := map[string]Config{
		"missing db":   {JWTSecret: "01234567890123456789012345678901", SMTPFrom: "a@b.c"},
		"short secret": {DBSource: "x", JWTSecret: "short", SMTPFrom: "a@b.c"},
		"missing from": {DBSource: "x", JWTSecret: "01234567890123456789012345678901"},
	}
	for name, cfg := range tests {
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
```

## Step 2: Run test, verify it FAILS
`go test ./internal/config/ -run TestValidate -v` → FAIL (does not compile).

## Step 3: Write `internal/config/config.go`
```go
package config

import (
	"errors"
	"time"

	"github.com/urfave/cli/v3"
)

type Config struct {
	HTTPAddr        string
	DBSource        string
	JWTSecret       string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	RiverMaxWorkers int
}

func (c Config) Validate() error {
	if c.DBSource == "" {
		return errors.New("db-source is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("jwt-secret must be at least 32 characters")
	}
	if c.SMTPFrom == "" {
		return errors.New("smtp-from is required")
	}
	return nil
}

func Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "http-addr", Value: ":8080", Sources: cli.EnvVars("HTTP_ADDR")},
		&cli.StringFlag{Name: "db-source", Sources: cli.EnvVars("DB_SOURCE")},
		&cli.StringFlag{Name: "jwt-secret", Sources: cli.EnvVars("JWT_SECRET")},
		&cli.DurationFlag{Name: "access-ttl", Value: 15 * time.Minute, Sources: cli.EnvVars("ACCESS_TTL")},
		&cli.DurationFlag{Name: "refresh-ttl", Value: 24 * time.Hour, Sources: cli.EnvVars("REFRESH_TTL")},
		&cli.StringFlag{Name: "smtp-host", Sources: cli.EnvVars("SMTP_HOST")},
		&cli.IntFlag{Name: "smtp-port", Value: 1025, Sources: cli.EnvVars("SMTP_PORT")},
		&cli.StringFlag{Name: "smtp-username", Sources: cli.EnvVars("SMTP_USERNAME")},
		&cli.StringFlag{Name: "smtp-password", Sources: cli.EnvVars("SMTP_PASSWORD")},
		&cli.StringFlag{Name: "smtp-from", Sources: cli.EnvVars("SMTP_FROM")},
		&cli.IntFlag{Name: "river-max-workers", Value: 10, Sources: cli.EnvVars("RIVER_MAX_WORKERS")},
	}
}

func FromCommand(cmd *cli.Command) Config {
	return Config{
		HTTPAddr:        cmd.String("http-addr"),
		DBSource:        cmd.String("db-source"),
		JWTSecret:       cmd.String("jwt-secret"),
		AccessTTL:       cmd.Duration("access-ttl"),
		RefreshTTL:      cmd.Duration("refresh-ttl"),
		SMTPHost:        cmd.String("smtp-host"),
		SMTPPort:        cmd.Int("smtp-port"),
		SMTPUsername:    cmd.String("smtp-username"),
		SMTPPassword:    cmd.String("smtp-password"),
		SMTPFrom:        cmd.String("smtp-from"),
		RiverMaxWorkers: cmd.Int("river-max-workers"),
	}
}
```

IMPORTANT compatibility note: In the installed urfave/cli v3, `cmd.Int(...)` and `cli.IntFlag` may use `int64` rather than `int`. If the code does not compile because of this, change the `SMTPPort` and `RiverMaxWorkers` struct fields to `int64` (and keep `FromCommand` assignments matching), OR cast with `int(cmd.Int(...))`. Pick whichever keeps the test compiling and `go build ./...` clean. Verify the exact `cli.EnvVars` / `Sources` field names against the installed v3 API and adjust if needed — the required behavior is: each flag falls back to the named env var.

## Step 4: Run test, verify it PASSES
`go test ./internal/config/ -run TestValidate -v` → PASS.

## Step 5: Commit
```bash
git add internal/config/
git commit -m "feat: add config with cli flags and env value sources"
```

## Global Constraints
- Module path `github.com/vancanhuit/simplebank`, Go `1.26.5`.
- Config loaded via urfave/cli v3 flags, each with an env value-source fallback.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-2-report.md`. Return only: status, commit hash(es), one-line test summary, concerns.
