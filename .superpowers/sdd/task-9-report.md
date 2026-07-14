# Task 9 Report: JWT token maker

**Status:** DONE
**Commit:** `ba4a37949a536ca18b12f8ddd2e6442291c601f8`

## Files
- Created `internal/token/maker.go` — `Payload` (embeds `jwt.RegisteredClaims`), `NewPayload`, `ErrExpiredToken`, `ErrInvalidToken`, `Maker` interface.
- Created `internal/token/jwt_maker.go` — `JWTMaker`, `NewJWTMaker` (rejects keys `< 32`), `CreateToken` (HS256), `VerifyToken` (HS256-only via `WithValidMethods` + HMAC method check).
- Created `internal/token/jwt_maker_test.go` — three tests (verbatim from brief).

## TDD flow
1. Wrote test → ran `go test ./internal/token/ -v` → FAIL (undefined types), as expected.
2. Wrote `maker.go` and `jwt_maker.go`.
3. Ran `go test ./internal/token/ -v` → PASS (all three).
4. `go build ./...` clean; `go vet ./...` clean.
5. Committed with the exact conventional message.

## Test summary
`go test ./internal/token/ -v` → PASS: TestJWTMaker, TestJWTMakerExpired, TestJWTMakerInvalidAlg (3/3).

## Constraints honored
- Uses `golang-jwt/jwt/v5` and `google/uuid` (from Task 1).
- Secret min length 32 enforced in `NewJWTMaker`.
- HS256 only; other algorithms rejected → `ErrInvalidToken`.
- Expired tokens → `ErrExpiredToken`.
- No tokens/secrets logged.
- Signatures and `Payload` shape kept exactly as specified for api/echo-jwt consumers (Task 13).

## Concerns
None.
