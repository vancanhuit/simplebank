# Task 3 Report: Utility helpers

## Status: DONE

## Summary
Implemented password hashing, random string, and currency helpers under `internal/util/` using test-driven development.

## Files created
- `internal/util/password_test.go` (test)
- `internal/util/currency_test.go` (test)
- `internal/util/password.go` — `HashPassword`, `CheckPassword` (bcrypt cost 12)
- `internal/util/random.go` — `RandomString`, `RandomOwner`
- `internal/util/currency.go` — `USD`/`EUR`/`VND` consts, `IsSupportedCurrency`

## TDD steps
1. Wrote both test files. (Both initially contained a duplicate `package util` line — a transient artifact from file creation — which was removed before proceeding.)
2. `go test ./internal/util/ -v` → FAIL (undefined: HashPassword, CheckPassword, RandomString, IsSupportedCurrency, USD). Confirmed red.
3. Wrote the three implementation files verbatim from the brief.
4. `go test ./internal/util/ -v` → PASS.
   ```
   --- PASS: TestIsSupportedCurrency (0.00s)
   --- PASS: TestPassword (0.99s)
   ok  github.com/vancanhuit/simplebank/internal/util  0.997s
   ```
5. `go build ./...` → clean.
6. `go vet ./...` → clean.
7. Committed.

## Verification
- `go test ./internal/util/ -v`: 2/2 tests PASS
- `go build ./...`: clean
- `go vet ./...`: clean
- bcrypt cost = 12 (>= 12 requirement met)
- No secrets/passwords logged
- Signatures match the brief exactly

## Concerns
None.
