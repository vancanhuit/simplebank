# Task 13: User/auth/account/transfer handlers + routes + auth middleware

**Files:**
- Create: `internal/api/middleware.go`
- Create: `internal/api/user.go`
- Create: `internal/api/account.go`
- Create: `internal/api/transfer.go`
- Create: `internal/api/routes.go`
- Modify: `internal/api/server.go` (REMOVE the stub `registerRoutes` — it moves to routes.go; KEEP `livez`)
- Modify: `internal/api/validator.go` (use `http.StatusBadRequest` instead of magic `400`)

## Confirmed facts (use verbatim)
- Echo v5 handler signature: `func(c *echo.Context) error`.
- Retrieve JWT token in handlers: `echo.ContextGet[*jwt.Token](c, "user")` (import `jwt "github.com/golang-jwt/jwt/v5"`).
- echo-jwt v5: `echojwt.WithConfig(echojwt.Config{SigningKey: []byte(secret), ContextKey: "user", NewClaimsFunc: func(c *echo.Context) jwt.Claims { return new(token.Payload) }})`.
- Generated sqlc names (Task 5): `CreateSessionParams` client-ip field is **`ClientIp`** (lowercase p); `Session.ExpiresAt` is `time.Time`; `ListAccountsParams{Owner string, Limit int32, Offset int32}`; `AddAccountBalanceParams{Amount int64, ID uuid.UUID}`.
- `token.Payload` has `.ID uuid.UUID`, `.Username`, `.Role`, and embeds `jwt.RegisteredClaims` (so `.ExpiresAt.Time` works).
- `store.ClassifyError`, `store.ErrRecordNotFound`, `store.TransferTx`, `store.TransferTxParams` exist.

## Step 1: `internal/api/validator.go` fix
Change `echo.NewHTTPError(400, err.Error())` to `echo.NewHTTPError(http.StatusBadRequest, "invalid request payload")` and add the `net/http` import. (This both removes the magic number and stops leaking validator struct field names to clients.)

## Step 2: `internal/api/middleware.go`
```go
package api

import (
	jwt "github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v5"
	"github.com/labstack/echo/v5"

	"github.com/vancanhuit/simplebank/internal/token"
)

const authContextKey = "user"

func (s *Server) authMiddleware() echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey: []byte(s.config.JWTSecret),
		ContextKey: authContextKey,
		NewClaimsFunc: func(c *echo.Context) jwt.Claims {
			return new(token.Payload)
		},
	})
}

func authPayload(c *echo.Context) (*token.Payload, error) {
	jwtToken, err := echo.ContextGet[*jwt.Token](c, authContextKey)
	if err != nil {
		return nil, echo.ErrUnauthorized
	}
	payload, ok := jwtToken.Claims.(*token.Payload)
	if !ok {
		return nil, echo.ErrUnauthorized
	}
	return payload, nil
}
```
VERIFY echo-jwt v5 `Config` field names (`SigningKey`, `ContextKey`, `NewClaimsFunc`) against the installed version and that `echo.ContextGet[*jwt.Token]` exists in Echo v5.3.0. Adjust if needed but keep behavior: middleware validates a Bearer HS256 token and stores it under "user"; `authPayload` extracts `*token.Payload`.

## Step 3: `internal/api/user.go`
(Full code — createUser, loginUser, renewToken, verifyEmail.)
```go
package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
	"github.com/vancanhuit/simplebank/internal/worker"
)

type createUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required,min=6"`
	FullName string `json:"full_name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
}

type userResponse struct {
	Username        string    `json:"username"`
	FullName        string    `json:"full_name"`
	Email           string    `json:"email"`
	IsEmailVerified bool      `json:"is_email_verified"`
	CreatedAt       time.Time `json:"created_at"`
}

func newUserResponse(u sqlcdb.User) userResponse {
	return userResponse{
		Username:        u.Username,
		FullName:        u.FullName,
		Email:           u.Email,
		IsEmailVerified: u.IsEmailVerified,
		CreatedAt:       u.CreatedAt,
	}
}

func (s *Server) createUser(c *echo.Context) error {
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	hashed, err := util.HashPassword(req.Password)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.CreateUser(ctx, sqlcdb.CreateUserParams{
		Username:       req.Username,
		HashedPassword: hashed,
		FullName:       req.FullName,
		Email:          req.Email,
	})
	if err != nil {
		return store.ClassifyError(err)
	}

	if _, err := s.riverClient.Insert(ctx, worker.SendVerifyEmailArgs{Username: user.Username}, nil); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, newUserResponse(user))
}

type loginUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required"`
}

type loginUserResponse struct {
	AccessToken           string       `json:"access_token"`
	AccessTokenExpiresAt  time.Time    `json:"access_token_expires_at"`
	RefreshToken          string       `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time    `json:"refresh_token_expires_at"`
	SessionID             string       `json:"session_id"`
	User                  userResponse `json:"user"`
}

func (s *Server) loginUser(c *echo.Context) error {
	var req loginUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.GetUser(ctx, req.Username)
	if err != nil {
		if e := store.ClassifyError(err); e == store.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}
	if err := util.CheckPassword(req.Password, user.HashedPassword); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	accessToken, accessPayload, err := s.tokenMaker.CreateToken(user.Username, "depositor", s.config.AccessTTL)
	if err != nil {
		return err
	}
	refreshToken, refreshPayload, err := s.tokenMaker.CreateToken(user.Username, "depositor", s.config.RefreshTTL)
	if err != nil {
		return err
	}

	session, err := s.store.CreateSession(ctx, sqlcdb.CreateSessionParams{
		ID:           refreshPayload.ID,
		Username:     user.Username,
		RefreshToken: refreshToken,
		UserAgent:    c.Request().UserAgent(),
		ClientIp:     c.RealIP(),
		IsBlocked:    false,
		ExpiresAt:    refreshPayload.ExpiresAt.Time,
	})
	if err != nil {
		return store.ClassifyError(err)
	}

	return c.JSON(http.StatusOK, loginUserResponse{
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessPayload.ExpiresAt.Time,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshPayload.ExpiresAt.Time,
		SessionID:             session.ID.String(),
		User:                  newUserResponse(user),
	})
}

type renewTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (s *Server) renewToken(c *echo.Context) error {
	var req renewTokenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	refreshPayload, err := s.tokenMaker.VerifyToken(req.RefreshToken)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	session, err := s.store.GetSession(ctx, refreshPayload.ID)
	if err != nil {
		return store.ClassifyError(err)
	}
	if session.IsBlocked ||
		session.Username != refreshPayload.Username ||
		session.RefreshToken != req.RefreshToken ||
		time.Now().After(session.ExpiresAt) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}

	accessToken, accessPayload, err := s.tokenMaker.CreateToken(refreshPayload.Username, refreshPayload.Role, s.config.AccessTTL)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"access_token":            accessToken,
		"access_token_expires_at": accessPayload.ExpiresAt.Time,
	})
}

func (s *Server) verifyEmail(c *echo.Context) error {
	idStr := c.QueryParam("id")
	code := c.QueryParam("code")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	ctx := c.Request().Context()
	ve, err := s.store.UpdateVerifyEmail(ctx, sqlcdb.UpdateVerifyEmailParams{
		ID:         id,
		SecretCode: code,
	})
	if err != nil {
		if e := store.ClassifyError(err); e == store.ErrRecordNotFound {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired verification link")
		}
		return err
	}
	if _, err := s.store.VerifyUserEmail(ctx, ve.Username); err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusOK, map[string]bool{"is_verified": true})
}
```
VERIFY: River client `Insert(ctx, args, *river.InsertOpts)` signature (nil opts). `sqlcdb.User` fields (`Username`, `FullName`, `Email`, `IsEmailVerified`, `CreatedAt`, `HashedPassword`). `Session.ExpiresAt time.Time`. Adjust if names differ.

## Step 4: `internal/api/account.go`
```go
package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/util"
)

type createAccountRequest struct {
	Currency string `json:"currency" validate:"required"`
}

func (s *Server) createAccount(c *echo.Context) error {
	var req createAccountRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}
	if !util.IsSupportedCurrency(req.Currency) {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported currency")
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}

	account, err := s.store.CreateAccount(c.Request().Context(), sqlcdb.CreateAccountParams{
		Owner:    payload.Username,
		Balance:  0,
		Currency: req.Currency,
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusCreated, account)
}

func (s *Server) getAccount(c *echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid account id")
	}

	account, err := s.store.GetAccount(c.Request().Context(), id)
	if err != nil {
		return store.ClassifyError(err)
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	if account.Owner != payload.Username {
		return echo.NewHTTPError(http.StatusForbidden, "account does not belong to you")
	}
	return c.JSON(http.StatusOK, account)
}

func (s *Server) listAccounts(c *echo.Context) error {
	page, err := echo.QueryParamOr[int32](c, "page", 1)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid page")
	}
	size, err := echo.QueryParamOr[int32](c, "size", 5)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid size")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 5
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}

	accounts, err := s.store.ListAccounts(c.Request().Context(), sqlcdb.ListAccountsParams{
		Owner:  payload.Username,
		Limit:  size,
		Offset: (page - 1) * size,
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusOK, accounts)
}
```
VERIFY `echo.QueryParamOr[int32]` exists in Echo v5.3.0 (it does — generic query binder). `ListAccountsParams.Limit/Offset` are `int32` (confirmed).

## Step 5: `internal/api/transfer.go`
```go
package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type transferRequest struct {
	FromAccountID string `json:"from_account_id" validate:"required,uuid"`
	ToAccountID   string `json:"to_account_id" validate:"required,uuid"`
	Amount        int64  `json:"amount" validate:"required,gt=0"`
	Currency      string `json:"currency" validate:"required"`
}

func (s *Server) createTransfer(c *echo.Context) error {
	var req transferRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	fromID, _ := uuid.Parse(req.FromAccountID)
	toID, _ := uuid.Parse(req.ToAccountID)
	ctx := c.Request().Context()

	fromAccount, err := s.validAccount(ctx, fromID, req.Currency)
	if err != nil {
		return err
	}
	if _, err := s.validAccount(ctx, toID, req.Currency); err != nil {
		return err
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	if fromAccount.Owner != payload.Username {
		return echo.NewHTTPError(http.StatusForbidden, "from account does not belong to you")
	}

	result, err := s.store.TransferTx(ctx, store.TransferTxParams{
		FromAccountID: fromID,
		ToAccountID:   toID,
		Amount:        req.Amount,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) validAccount(ctx context.Context, id uuid.UUID, currency string) (sqlcdb.Account, error) {
	account, err := s.store.GetAccount(ctx, id)
	if err != nil {
		return account, store.ClassifyError(err)
	}
	if account.Currency != currency {
		return account, echo.NewHTTPError(http.StatusBadRequest, "currency mismatch")
	}
	return account, nil
}
```

## Step 6: `internal/api/routes.go` + remove stub from server.go
Create `internal/api/routes.go`:
```go
package api

func (s *Server) registerRoutes() {
	s.router.GET("/livez", s.livez)

	v1 := s.router.Group("/api/v1")

	v1.POST("/users", s.createUser)
	v1.POST("/users/login", s.loginUser)
	v1.POST("/tokens/renew", s.renewToken)
	v1.GET("/users/verify_email", s.verifyEmail)

	auth := v1.Group("")
	auth.Use(s.authMiddleware())
	auth.POST("/accounts", s.createAccount)
	auth.GET("/accounts/:id", s.getAccount)
	auth.GET("/accounts", s.listAccounts)
	auth.POST("/transfers", s.createTransfer)
}
```
Then DELETE the `registerRoutes` method from `server.go` (keep `livez`, `NewServer`, `Handler`, struct). server.go's `NewServer` still calls `s.registerRoutes()` — that now resolves to routes.go. Ensure server.go no longer defines `registerRoutes` (only routes.go does) to avoid a duplicate-method compile error.

## Step 7: Verify build + existing test
`go build ./...`, `go vet ./...` → clean. `go test ./internal/api/ -v` → existing `TestToHTTPStatus` still PASS.

## Step 8: Commit
```bash
git add internal/api/
git commit -m "feat: add user, account, transfer handlers with JWT auth routes"
```

## Global Constraints
- JSON APIs under `/api/v1`; `/livez` at root.
- Handlers thin: bind/validate DTO, call store/service, map errors → HTTP. No pgx types leak into api.
- Owner authorization on account/transfer (403 if not owner).
- Money `int64`; never floats. Never log secrets/tokens.
- Access token short TTL, refresh stored in sessions (block/expiry checked on renew).
- Conventional commit message.

## Report contract
Write full report to `.superpowers/sdd/task-13-report.md`, noting any Echo/echo-jwt/River API adjustments and confirming `registerRoutes` exists in exactly ONE file. Return only: status, commit hash(es), one-line test/build summary, concerns.
