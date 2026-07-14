# Task 15: Auth rate limiting and API handler test

**Files:**
- Modify: `internal/api/routes.go`
- Create: `internal/api/user_test.go`

## Produces
- Per-IP rate limiting on the login and token-renew routes.
- A handler test proving `createUser` returns 400 on an invalid body (validation) without needing the DB.

## Step 1: Add rate limiter to auth-sensitive routes in `routes.go`
The current `routes.go` `registerRoutes` registers `POST /users/login` and `POST /tokens/renew` plainly. Wrap ONLY those two with a rate limiter middleware. Register each route exactly once (do not double-register).

Target shape:
```go
package api

import (
	"github.com/labstack/echo/v5/middleware"
	"golang.org/x/time/rate"
)

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)

	v1 := s.router.Group("/api/v1")

	authLimiter := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rate.Limit(5)))

	v1.POST("/users", s.createUser)
	v1.POST("/users/login", s.loginUser, authLimiter)
	v1.POST("/tokens/renew", s.renewToken, authLimiter)
	v1.GET("/users/verify_email", s.verifyEmail)

	auth := v1.Group("")
	auth.Use(s.authMiddleware())
	auth.POST("/accounts", s.createAccount)
	auth.GET("/accounts/:id", s.getAccount)
	auth.GET("/accounts", s.listAccounts)
	auth.POST("/transfers", s.createTransfer)
}
```
COMPATIBILITY — verify against Echo v5.3.0 `github.com/labstack/echo/v5/middleware`: the Rate Limiter middleware constructor and store. It may be `middleware.RateLimiter(store)` with `middleware.NewRateLimiterMemoryStore(rate.Limit)` OR `middleware.RateLimiterWithConfig(...)`. Read the installed middleware package (`go doc github.com/labstack/echo/v5/middleware` and grep for RateLimiter) and use the actual identifiers. REQUIRED BEHAVIOR: a per-client (per-IP) in-memory rate limit applied only to the login and token-renew routes (a modest rate like 5 req/s is fine). If the memory store constructor takes a config struct instead of a bare `rate.Limit`, use that form. Route-level middleware is passed as trailing args to `v1.POST(path, handler, mw...)` in Echo v5 (confirm).

Keep the existing behavior of all other routes unchanged and ensure each route is registered exactly once.

## Step 2: Create `internal/api/user_test.go`
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vancanhuit/simplebank/internal/config"
	"github.com/vancanhuit/simplebank/internal/token"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	maker, err := token.NewJWTMaker("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(config.Config{JWTSecret: "01234567890123456789012345678901"}, nil, maker, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateUserBadRequest(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
```
NOTE: this test passes `nil` for the store and river client. It works because validation of the invalid body fails and returns 400 BEFORE any store/river call. Confirm `NewServer` does not dereference the store/river client at construction time (it should not — it only stores them). If `NewServer` or route registration panics on nil, adjust the test to pass minimal non-nil fakes, but do NOT change production code to accommodate the test.

## Step 3: Run test
`go test ./internal/api/ -run TestCreateUserBadRequest -v` → PASS (400 from validation). Also `go test ./internal/api/ -v` (existing TestToHTTPStatus still passes), `go build ./...`, `go vet ./...`, `mise run golangci-lint` clean.

## Step 4: Commit
```bash
git add internal/api/routes.go internal/api/user_test.go
git commit -m "feat: add auth rate limiting and user handler validation test"
```

## Global Constraints
- Rate limiting on auth endpoints (`golang.org/x/time/rate`, added in Task 1).
- Each route registered exactly once.
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-15-report.md`, noting the exact Echo rate-limiter API used. Return only: status, commit hash(es), one-line test summary, concerns.
