# Task 12: API server scaffold, DTO validation, and error handler

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/errors.go`
- Create: `internal/api/validator.go`
- Test: `internal/api/errors_test.go`

## Produces
- `type Server struct { config config.Config; store store.Store; tokenMaker token.Maker; riverClient *river.Client[pgx.Tx]; router *echo.Echo }`
- `func NewServer(cfg config.Config, st store.Store, maker token.Maker, riverClient *river.Client[pgx.Tx]) (*Server, error)`
- `func (s *Server) Handler() *echo.Echo`
- `func errorHandler(c *echo.Context, err error)`; `func toHTTPStatus(err error) int`
- `func (s *Server) registerRoutes()` — for THIS task, register ONLY `/livez` (health). Do NOT register `/readyz` here (it is wired with the DB pool in Task 14). Do NOT define a readyz handler.

IMPORTANT structural note: Task 13 will REPLACE `registerRoutes` with the full route set (adding `/api/v1` routes). So keep `registerRoutes` minimal and in `server.go` for now; Task 13 moves/expands it.

## Step 0: Add validator dependency
`go get github.com/go-playground/validator/v10@latest` (do NOT run `go mod tidy`).

## Step 1: Write failing test `internal/api/errors_test.go`
```go
package api

import (
	"net/http"
	"testing"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func TestToHTTPStatus(t *testing.T) {
	cases := map[error]int{
		store.ErrRecordNotFound:      http.StatusNotFound,
		store.ErrUniqueViolation:     http.StatusConflict,
		store.ErrForeignKeyViolation: http.StatusConflict,
		store.ErrInsufficientBalance: http.StatusUnprocessableEntity,
		token.ErrExpiredToken:        http.StatusUnauthorized,
		token.ErrInvalidToken:        http.StatusUnauthorized,
	}
	for err, want := range cases {
		if got := toHTTPStatus(err); got != want {
			t.Errorf("%v: got %d want %d", err, got, want)
		}
	}
}
```

## Step 2: Run test, verify FAIL
`go test ./internal/api/ -run TestToHTTPStatus -v` → FAIL (does not compile).

## Step 3: Write `internal/api/errors.go`
```go
package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

func toHTTPStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrRecordNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrUniqueViolation), errors.Is(err, store.ErrForeignKeyViolation):
		return http.StatusConflict
	case errors.Is(err, store.ErrInsufficientBalance):
		return http.StatusUnprocessableEntity
	case errors.Is(err, token.ErrExpiredToken), errors.Is(err, token.ErrInvalidToken):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func errorHandler(c *echo.Context, err error) {
	if c.Response().Committed {
		return
	}

	var he *echo.HTTPError
	if errors.As(err, &he) {
		_ = c.JSON(he.StatusCode(), map[string]string{"error": he.Message})
		return
	}

	status := toHTTPStatus(err)
	message := "internal server error"
	if status != http.StatusInternalServerError {
		message = err.Error()
	} else {
		c.Logger().Error("request failed", "error", err)
	}
	_ = c.JSON(status, map[string]string{"error": message})
}
```
VERIFY against Echo v5.3.0: `echo.HTTPError` has `StatusCode()` and a `Message` field (string). `c.Response().Committed` and `c.Logger()` (returns `*slog.Logger`). Adjust if the installed signatures differ (e.g. `he.Message` may be `any` — if so, format with `fmt.Sprintf("%v", he.Message)`).

## Step 4: Write `internal/api/validator.go`
```go
package api

import (
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
)

type customValidator struct {
	validate *validator.Validate
}

func (cv *customValidator) Validate(i any) error {
	if err := cv.validate.Struct(i); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	return nil
}

func newValidator() *customValidator {
	return &customValidator{validate: validator.New()}
}
```

## Step 5: Write `internal/api/server.go`
```go
package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/riverqueue/river"

	"github.com/vancanhuit/simplebank/internal/config"
	store "github.com/vancanhuit/simplebank/internal/db"
	"github.com/vancanhuit/simplebank/internal/token"
)

type Server struct {
	config      config.Config
	store       store.Store
	tokenMaker  token.Maker
	riverClient *river.Client[pgx.Tx]
	router      *echo.Echo
}

func NewServer(
	cfg config.Config,
	st store.Store,
	maker token.Maker,
	riverClient *river.Client[pgx.Tx],
) (*Server, error) {
	e := echo.NewWithConfig(echo.Config{
		HTTPErrorHandler: errorHandler,
		Validator:        newValidator(),
	})

	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(1 << 20))
	e.Use(middleware.ContextTimeout(30 * time.Second))
	e.Use(middleware.Recover())

	s := &Server{
		config:      cfg,
		store:       st,
		tokenMaker:  maker,
		riverClient: riverClient,
		router:      e,
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() *echo.Echo { return s.router }

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)
}

func (s *Server) livez(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
```
VERIFY against Echo v5.3.0: `echo.NewWithConfig(echo.Config{...})` with fields `HTTPErrorHandler` (type `func(*echo.Context, error)`) and `Validator`. The middleware names `middleware.RequestID/RequestLogger/Secure/BodyLimit/ContextTimeout/Recover` exist (they are used in the current `cmd/app/main.go` — check there). Handler signature is `func(c *echo.Context) error`. Adjust import/config field names if the installed API differs.

## Step 6: Run test, verify PASS
`go test ./internal/api/ -run TestToHTTPStatus -v` → PASS. Also `go build ./...` and `go vet ./...` clean.

## Step 7: Commit
```bash
git add internal/api/ go.mod go.sum
git commit -m "feat: add api server scaffold, error handler, and validator"
```

## Global Constraints
- Echo v5 handler signature `func(c *echo.Context) error`.
- Error handler maps app errors → 400/401/403/404/409/422/500; never leaks internal detail (500 → generic message + logged).
- Layered timeout via `middleware.ContextTimeout(30s)`.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-12-report.md`, noting any Echo API adjustments (esp. `HTTPError.Message` type, error-handler signature). Return only: status, commit hash(es), one-line test summary, concerns.
