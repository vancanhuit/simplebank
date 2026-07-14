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
	if fromID == toID {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot transfer to the same account")
	}
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
