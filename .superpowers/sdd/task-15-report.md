# Task 15 Report: Auth rate limiting and API handler test

**Status:** DONE

## Changes
- `internal/api/routes.go`: added a per-IP in-memory rate limiter applied only to `POST /users/login` and `POST /tokens/renew`. Each route registered exactly once.
- `internal/api/user_test.go`: added `newTestServer` helper and `TestCreateUserBadRequest` proving `createUser` returns 400 on an invalid body via validation, with `nil` store and river client.

## Exact Echo v5.3.0 rate-limiter API used
Verified against the installed package `github.com/labstack/echo/v5/middleware` via `go doc`:

- `func RateLimiter(store RateLimiterStore) echo.MiddlewareFunc`
- `func NewRateLimiterMemoryStore(rateLimit float64) (store *RateLimiterMemoryStore)` — takes a plain `float64` (req/s), Burst/ExpiresIn default.

Final code:
```go
authLimiter := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(5))
v1.POST("/users/login", s.loginUser, authLimiter)
v1.POST("/tokens/renew", s.renewToken, authLimiter)
```

Route-level middleware is passed as trailing args to `v1.POST(path, handler, mw...)`, confirmed by the `go doc` example.

### Deviation from brief target shape
The brief's target used `middleware.NewRateLimiterMemoryStore(rate.Limit(5))` and imported `golang.org/x/time/rate`. The actual v5.3.0 constructor signature takes `float64`, not `rate.Limit`. `rate.Limit` is a named type (underlying `float64`) and is NOT assignable to a `float64` parameter without explicit conversion, so `rate.Limit(5)` would fail to compile. I used a bare `5` and dropped the unused `golang.org/x/time/rate` import from `routes.go`. The default memory store is per-client (per-IP) via `DefaultRateLimiterMemoryStoreConfig` sourcing the identifier from `RealIP`.

## Verification
- `go test ./internal/api/ -v` → PASS: `TestToHTTPStatus`, `TestCreateUserBadRequest` (400 from validation).
- `go build ./...` → OK
- `go vet ./...` → OK
- `mise run golangci-lint` → 0 issues (task scope `./cmd/...`); also ran `golangci-lint run ./internal/api/...` directly → 0 issues.

`go mod tidy` was NOT run.
