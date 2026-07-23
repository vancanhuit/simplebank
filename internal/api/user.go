package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
	"github.com/vancanhuit/simplebank/internal/password"
	"github.com/vancanhuit/simplebank/internal/random"
	"github.com/vancanhuit/simplebank/internal/token"
	"github.com/vancanhuit/simplebank/internal/worker"
)

// dummyPasswordHash is compared against on the unknown-user login path so the
// response time does not reveal whether a username exists (mitigates user
// enumeration). Computed once at startup from a random value.
var dummyPasswordHash = func() string {
	h, err := password.Hash(random.String(16))
	if err != nil {
		panic(err)
	}
	return h
}()

// hashRefreshToken returns the value stored in sessions.refresh_token. Storing a
// hash instead of the raw token means a database leak does not yield usable
// refresh tokens.
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type createUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required,min=6,maxbytes=72"`
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
	req, err := bindValidate[createUserRequest](c)
	if err != nil {
		return err
	}

	hashed, err := password.Hash(req.Password)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.CreateUserTx(ctx, store.CreateUserTxParams{
		CreateUserParams: sqlcdb.CreateUserParams{
			Username:       req.Username,
			HashedPassword: hashed,
			FullName:       req.FullName,
			Email:          req.Email,
		},
		AfterCreate: func(tx pgx.Tx, u sqlcdb.User) error {
			_, err := s.riverClient.InsertTx(ctx, tx, worker.SendVerifyEmailArgs{Username: u.Username}, nil)
			return err
		},
	})
	if err != nil {
		return store.ClassifyError(err)
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
	req, err := bindValidate[loginUserRequest](c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.GetUser(ctx, req.Username)
	if err != nil {
		if errors.Is(store.ClassifyError(err), store.ErrRecordNotFound) {
			// Run a comparison against a dummy hash so an unknown username takes
			// the same time as a wrong password (no enumeration via timing).
			_ = password.Check(req.Password, dummyPasswordHash)
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
		}
		return err
	}
	if err := password.Check(req.Password, user.HashedPassword); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}

	tokens, err := s.issueTokenPair(user.Username, roleDepositor)
	if err != nil {
		return err
	}

	session, err := s.store.CreateSession(ctx, sqlcdb.CreateSessionParams{
		ID:           tokens.refreshPayload.ID,
		Username:     user.Username,
		RefreshToken: hashRefreshToken(tokens.refresh),
		UserAgent:    c.Request().UserAgent(),
		ClientIp:     c.RealIP(),
		IsBlocked:    false,
		ExpiresAt:    tokens.refreshPayload.ExpiresAt.Time,
	})
	if err != nil {
		return store.ClassifyError(err)
	}

	return c.JSON(http.StatusOK, loginUserResponse{
		AccessToken:           tokens.access,
		AccessTokenExpiresAt:  tokens.accessPayload.ExpiresAt.Time,
		RefreshToken:          tokens.refresh,
		RefreshTokenExpiresAt: tokens.refreshPayload.ExpiresAt.Time,
		SessionID:             session.ID.String(),
		User:                  newUserResponse(user),
	})
}

// tokenPair holds a freshly-issued access/refresh token and their payloads.
type tokenPair struct {
	access         string
	accessPayload  *token.Payload
	refresh        string
	refreshPayload *token.Payload
}

// issueTokenPair mints an access and a refresh token for the given identity,
// each with its configured TTL.
func (s *Server) issueTokenPair(username, role string) (tokenPair, error) {
	access, accessPayload, err := s.tokenMaker.CreateToken(username, role, s.config.AccessTTL)
	if err != nil {
		return tokenPair{}, err
	}
	refresh, refreshPayload, err := s.tokenMaker.CreateToken(username, role, s.config.RefreshTTL)
	if err != nil {
		return tokenPair{}, err
	}
	return tokenPair{
		access:         access,
		accessPayload:  accessPayload,
		refresh:        refresh,
		refreshPayload: refreshPayload,
	}, nil
}

type renewTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (s *Server) renewToken(c *echo.Context) error {
	req, err := bindValidate[renewTokenRequest](c)
	if err != nil {
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
	tokenMatch := subtle.ConstantTimeCompare(
		[]byte(session.RefreshToken), []byte(hashRefreshToken(req.RefreshToken))) == 1
	usernameMatch := session.Username == refreshPayload.Username
	expired := time.Now().After(session.ExpiresAt)
	if session.IsBlocked || !usernameMatch || !tokenMatch || expired {
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
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired verification link")
	}

	ctx := c.Request().Context()
	_, err = s.store.VerifyEmailTx(ctx, store.VerifyEmailTxParams{
		ID:         id,
		SecretCode: code,
	})
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired verification link")
		}
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"is_verified": true})
}
