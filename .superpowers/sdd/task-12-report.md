# Task 12 Report: API server scaffold, DTO validation, and error handler

## Status: DONE

## Files created
- `internal/api/server.go` — `Server` struct, `NewServer`, `Handler`, `registerRoutes` (only `/livez`), `livez` handler.
- `internal/api/errors.go` — `toHTTPStatus`, `errorHandler`.
- `internal/api/validator.go` — `customValidator`, `newValidator`.
- `internal/api/errors_test.go` — `TestToHTTPStatus`.

## TDD flow
1. `go get github.com/go-playground/validator/v10@latest` → added `v10.30.3` (as indirect; not tidied per instructions).
2. Wrote `errors_test.go` → ran → FAIL (`undefined: toHTTPStatus`, build failed). ✓ expected red.
3. Wrote `errors.go`, `validator.go`, `server.go`.
4. Ran test → PASS. `go build ./...` clean. `go vet ./...` clean.

## Echo v5.3.0 API verification & adjustments

Verified against the installed `github.com/labstack/echo/v5@v5.3.0` source:

- `HTTPErrorHandler` signature is `func(c *Context, err error)` — matches brief. ✓
- `echo.HTTPError.Message` is a **`string`** field (not `any`), and `StatusCode() int` exists — so `he.Message` and `he.StatusCode()` used verbatim, no `fmt.Sprintf` needed. ✓
- `echo.NewHTTPError(code int, message string)` — matches brief's `echo.NewHTTPError(400, err.Error())`. ✓
- `echo.NewWithConfig(echo.Config{HTTPErrorHandler, Validator})` — both fields exist. ✓
- `c.Logger()` returns `*slog.Logger`; `c.Logger().Error("...", "error", err)` works. ✓
- Middleware `RequestID/RequestLogger/Secure/BodyLimit/ContextTimeout/Recover` and handler signature `func(c *echo.Context) error` — match `cmd/app/main.go`. ✓

### One adjustment required (committed check)
The brief's `c.Response().Committed` does **not** compile in v5.3.0: `c.Response()` returns a plain `http.ResponseWriter`, which has no `Committed` field. The v5 idiom (used by `DefaultHTTPErrorHandler` in the echo source) is:

```go
if r, _ := echo.UnwrapResponse(c.Response()); r != nil && r.Committed {
	return
}
```

`errorHandler` was adjusted to use `echo.UnwrapResponse(c.Response())` accordingly. No other changes to the brief's code.

## Verification results
- `go test ./internal/api/ -run TestToHTTPStatus -v` → PASS
- `go build ./...` → clean
- `go vet ./...` → clean

## Notes for downstream tasks
- `registerRoutes` currently registers only `/livez`. Task 13 will replace it with the full `/api/v1` route set; Task 14 wires `/readyz` with the DB pool.
- `validator/v10` is currently marked `// indirect` in `go.mod` (no `go mod tidy` per instructions); it becomes direct once handlers use bound/validated DTOs.

## Concerns
None.
