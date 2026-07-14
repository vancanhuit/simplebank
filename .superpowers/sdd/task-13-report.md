# Task 13 Report: User/auth/account/transfer handlers + routes + auth middleware

**Status:** DONE
**Commit:** `6f244aed9fdb3881ebdda42c3b79a46425bac1e7` — `feat: add user, account, transfer handlers with JWT auth routes`
**Branch:** `feat/simplebank-implementation`

## Files
- Created: `internal/api/middleware.go`, `internal/api/user.go`, `internal/api/account.go`, `internal/api/transfer.go`, `internal/api/routes.go`
- Modified: `internal/api/server.go` (removed stub `registerRoutes`; kept `NewServer`, `Handler`, `livez`, `Server` struct)
- Modified: `internal/api/validator.go` (`echo.NewHTTPError(400, err.Error())` → `echo.NewHTTPError(http.StatusBadRequest, "invalid request payload")`, added `net/http` import — removes magic number and stops leaking validator field names)

## registerRoutes uniqueness
Confirmed: `func (s *Server) registerRoutes` is defined in **exactly one file** — `internal/api/routes.go`. `grep -rln` over `internal/api/` returns only `routes.go`. server.go's `NewServer` still calls `s.registerRoutes()`, now resolving to routes.go.

## API verification against installed versions
Installed: echo v5.3.0, echo-jwt v5.0.2, golang-jwt/jwt v5.3.1, river v0.40.0. All flagged names verified as-is — **no code adjustments required**:

- **echo-jwt `Config`** (`jwt.go`): fields `SigningKey interface{}`, `ContextKey string`, `NewClaimsFunc func(c *echo.Context) jwt.Claims` all present → brief code compiles unchanged.
- **`echo.ContextGet[T any](c *Context, key string) (T, error)`** exists (`context_generic.go`) → `echo.ContextGet[*jwt.Token]` valid.
- **`echo.QueryParamOr[T any](c *Context, key string, defaultValue T, opts ...any) (T, error)`** exists (`binder_generic.go`) → `echo.QueryParamOr[int32]` valid.
- **River `Insert(ctx context.Context, args JobArgs, opts *InsertOpts)`** (`client.go`) → `Insert(ctx, args, nil)` valid.
- **`token.Payload`**: has `.ID uuid.UUID`, `.Username`, `.Role`, embeds `jwt.RegisteredClaims` (golang-jwt/v5) → satisfies `jwt.Claims`; `.ExpiresAt.Time` works.
- **`token.Maker.CreateToken(username, role string, duration) (string, *Payload, error)`** and `VerifyToken` match handler usage.
- `worker.SendVerifyEmailArgs{Username string}`, `config.{JWTSecret,AccessTTL,RefreshTTL}` confirmed.
- Generated sqlc fields (`CreateSessionParams.ClientIp`, `ListAccountsParams.Limit/Offset int32`, `Session.ExpiresAt time.Time`, `User` fields) — all compiled cleanly.

## Verification
- `go build ./...` → clean (BUILD_OK)
- `go vet ./...` → clean (VET_OK)
- `go test ./internal/api/ -v` → `TestToHTTPStatus` PASS; `ok ... 0.004s`

`go mod tidy` was NOT run, per instructions.

## Concerns
None. All flagged Echo/echo-jwt/River API names matched the installed versions verbatim; no behavioral adjustments were necessary.

---

## Follow-up hardening fixes

**Status:** DONE
**Commit:** `ba6e48e417e6aed85e9b253209331340a8cdd11e` — `fix: harden api - no query in logs, reject self-transfer, role const, clamp page size`
**Verification:** `go build ./...` OK, `go vet ./...` OK, `go test ./internal/api/ -v` PASS (TestToHTTPStatus).

### 1. Logger no longer logs the query string
- **Finding:** The default `middleware.RequestLogger()` calls `RequestLoggerWithConfig` with `LogURI: true` and logs `slog.String("uri", v.URI)`. `RequestLoggerValues.URI` is the full request URI **including the query string** (e.g. `/list?lang=en&page=1`), so the verify-email `?code=<secret>` value would be written to logs.
- **Echo middleware API used:** `middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{...})` with a mandatory custom `LogValuesFunc`. Disabled `LogURI`; enabled `LogURIPath: true` instead and log `slog.String("path", v.URIPath)` (path part only, e.g. `/list`). All other default fields (method, status, latency, host, bytes_in/out, user_agent, remote_ip, request_id) retained; on error the error string is added. `HandleError: true` preserved.
- **Change location:** Added `requestLogger()` helper in `internal/api/server.go`; swapped `e.Use(middleware.RequestLogger())` → `e.Use(requestLogger())`. Added `context` and `log/slog` imports.
- **Outcome:** The `code` query parameter value is never written to logs (only the matched path is logged).

### 2. Reject self-transfer
- `internal/api/transfer.go`: after parsing `fromID`/`toID`, added `if fromID == toID { return echo.NewHTTPError(http.StatusBadRequest, "cannot transfer to the same account") }`.

### 3. Role constant
- `internal/api/middleware.go`: added `roleDepositor = "depositor"` to the const block.
- `internal/api/user.go`: `loginUser` now uses `roleDepositor` in both `CreateToken` calls (access + refresh) instead of the `"depositor"` literals.

### 4. Clamp listAccounts size
- `internal/api/account.go`: replaced `if size < 1 || size > 100 { size = 5 }` with `if size < 1 { size = 5 }` and `if size > 100 { size = 100 }` so oversized requests clamp to 100 while undersized still default to 5.

**Note:** `getAccount` 403 behavior left unchanged per instructions.
