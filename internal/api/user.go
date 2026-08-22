package api

import (
	"context"
	"crypto/sha256"
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
	"github.com/vancanhuit/simplebank/internal/secret"
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

var verificationAccepted = map[string]string{
	"message": "check your email for verification instructions",
}

// hashRefreshToken returns the value stored in sessions.refresh_token. Storing a
// hash instead of the raw token means a database leak does not yield usable
// refresh tokens.
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type createUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required,min=15,maxbytes=72"`
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

func (s *Server) queueVerifyEmailTx(ctx context.Context, tx pgx.Tx, username string) error {
	if s.riverClient == nil {
		return nil
	}
	_, err := s.riverClient.InsertTx(ctx, tx, worker.SendVerifyEmailArgs{Username: username}, nil)
	return err
}

func (s *Server) queueVerifyEmail(ctx context.Context, username string) error {
	if s.riverClient == nil {
		return nil
	}
	_, err := s.riverClient.Insert(ctx, worker.SendVerifyEmailArgs{Username: username}, nil)
	return err
}

func (s *Server) queueRegistrationNotice(ctx context.Context, email string) error {
	if s.riverClient == nil {
		return nil
	}
	_, err := s.riverClient.Insert(ctx, worker.SendRegistrationNoticeArgs{Email: email}, nil)
	return err
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
	_, err = s.store.CreateUserTx(ctx, store.CreateUserTxParams{
		Username:       req.Username,
		HashedPassword: hashed,
		FullName:       req.FullName,
		Email:          req.Email,
		AfterCreate: func(tx pgx.Tx, u sqlcdb.User) error {
			return s.queueVerifyEmailTx(ctx, tx, u.Username)
		},
	})
	if err != nil {
		classified := store.ClassifyError(err)
		if errors.Is(classified, store.ErrUsernameExists) {
			return c.JSON(http.StatusAccepted, verificationAccepted)
		}
		if errors.Is(classified, store.ErrEmailExists) {
			if enqueueErr := s.queueRegistrationNotice(ctx, req.Email); enqueueErr != nil {
				c.Logger().Error("enqueue registration notice", "error", enqueueErr)
			}
			return c.JSON(http.StatusAccepted, verificationAccepted)
		}
		c.Logger().Error("create user transaction", "error", classified)
		return c.JSON(http.StatusAccepted, verificationAccepted)
	}

	return c.JSON(http.StatusAccepted, verificationAccepted)
}

type loginUserRequest struct {
	Username string `json:"username" validate:"required,alphanum"`
	Password string `json:"password" validate:"required"`
}

type loginUserResponse struct {
	AccessToken          string       `json:"access_token"`
	AccessTokenExpiresAt time.Time    `json:"access_token_expires_at"`
	SessionID            string       `json:"session_id"`
	User                 userResponse `json:"user"`
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
	if !user.IsEmailVerified {
		return echo.NewHTTPError(http.StatusForbidden, "email verification required")
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
	s.setRefreshCookie(c, tokens.refresh, tokens.refreshPayload.ExpiresAt.Time)

	return c.JSON(http.StatusOK, loginUserResponse{
		AccessToken:          tokens.access,
		AccessTokenExpiresAt: tokens.accessPayload.ExpiresAt.Time,
		SessionID:            session.ID.String(),
		User:                 newUserResponse(user),
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
	return s.issueTokenPairWithRefreshID(uuid.Nil, username, role)
}

func (s *Server) issueTokenPairWithRefreshID(refreshID uuid.UUID, username, role string) (tokenPair, error) {
	access, accessPayload, err := s.tokenMaker.CreateToken(username, role, token.Access, s.config.AccessTTL)
	if err != nil {
		return tokenPair{}, err
	}
	var refresh string
	var refreshPayload *token.Payload
	if refreshID == uuid.Nil {
		refresh, refreshPayload, err = s.tokenMaker.CreateToken(username, role, token.Refresh, s.config.RefreshTTL)
	} else {
		refresh, refreshPayload, err = s.tokenMaker.CreateTokenWithID(refreshID, username, role, token.Refresh, s.config.RefreshTTL)
	}
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

type renewTokenResponse struct {
	AccessToken          string       `json:"access_token"`
	AccessTokenExpiresAt time.Time    `json:"access_token_expires_at"`
	User                 userResponse `json:"user"`
}

func (s *Server) renewToken(c *echo.Context) error {
	refreshCookie, err := c.Cookie(refreshCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return c.NoContent(http.StatusNoContent)
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}

	refreshPayload, err := s.tokenMaker.VerifyToken(refreshCookie.Value, token.Refresh)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	var tokens tokenPair
	_, err = s.store.RotateSessionTx(ctx, store.RotateSessionTxParams{
		ID:               refreshPayload.ID,
		Username:         refreshPayload.Username,
		RefreshTokenHash: hashRefreshToken(refreshCookie.Value),
		Now:              time.Now(),
		NewSession: func() (store.SessionReplacement, error) {
			var issueErr error
			tokens, issueErr = s.issueTokenPairWithRefreshID(refreshPayload.ID, refreshPayload.Username, refreshPayload.Role)
			if issueErr != nil {
				return store.SessionReplacement{}, issueErr
			}
			return store.SessionReplacement{
				ID:               tokens.refreshPayload.ID,
				RefreshTokenHash: hashRefreshToken(tokens.refresh),
				ExpiresAt:        tokens.refreshPayload.ExpiresAt.Time,
			}, nil
		},
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	user, err := s.store.GetUser(ctx, refreshPayload.Username)
	if err != nil {
		return store.ClassifyError(err)
	}
	s.setRefreshCookie(c, tokens.refresh, tokens.refreshPayload.ExpiresAt.Time)

	return c.JSON(http.StatusOK, renewTokenResponse{
		AccessToken:          tokens.access,
		AccessTokenExpiresAt: tokens.accessPayload.ExpiresAt.Time,
		User:                 newUserResponse(user),
	})
}

func (s *Server) logoutUser(c *echo.Context) error {
	refreshCookie, err := c.Cookie(refreshCookieName)
	if err != nil {
		s.clearRefreshCookie(c)
		return c.NoContent(http.StatusNoContent)
	}

	refreshPayload, err := s.tokenMaker.VerifyToken(refreshCookie.Value, token.Refresh)
	if err != nil {
		s.clearRefreshCookie(c)
		return c.NoContent(http.StatusNoContent)
	}

	_, err = s.store.BlockSession(c.Request().Context(), refreshPayload.ID)
	if err != nil {
		classified := store.ClassifyError(err)
		if errors.Is(classified, store.ErrRecordNotFound) {
			// Missing or already-rotated sessions are normalized so logout does
			// not reveal whether a refresh session row exists.
			s.clearRefreshCookie(c)
			return c.NoContent(http.StatusNoContent)
		}
		s.clearRefreshCookie(c)
		return classified
	}

	s.clearRefreshCookie(c)
	return c.NoContent(http.StatusNoContent)
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
		SecretCode: secret.Digest(code),
	})
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired verification link")
		}
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"is_verified": true})
}

type resendVerifyEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
}

func (s *Server) resendVerifyEmail(c *echo.Context) error {
	req, err := bindValidate[resendVerifyEmailRequest](c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil {
		classified := store.ClassifyError(err)
		if errors.Is(classified, store.ErrRecordNotFound) {
			return c.JSON(http.StatusAccepted, verificationAccepted)
		}
		return classified
	}
	if user.IsEmailVerified {
		return c.JSON(http.StatusAccepted, verificationAccepted)
	}
	if enqueueErr := s.queueVerifyEmail(ctx, user.Username); enqueueErr != nil {
		c.Logger().Error("enqueue verify email", "error", enqueueErr)
	}
	return c.JSON(http.StatusAccepted, verificationAccepted)
}
