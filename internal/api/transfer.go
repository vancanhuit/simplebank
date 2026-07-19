package api

import (
	"context"
	"math"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type transferRequest struct {
	FromAccountID  string `json:"from_account_id" validate:"required,uuid"`
	ToAccountID    string `json:"to_account_id" validate:"required,uuid"`
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	Currency       string `json:"currency" validate:"required"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,uuid"`
}

func (s *Server) createTransfer(c *echo.Context) error {
	var req transferRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return err
	}

	// Transfer limits are per-currency: both accounts share req.Currency, so a
	// single lookup gives the ceilings in that currency's minor units.
	limit := s.config.LimitFor(req.Currency)
	if limit.MaxPerTransfer > 0 && req.Amount > limit.MaxPerTransfer {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "amount exceeds the per-transfer limit")
	}

	fromID, _ := uuid.Parse(req.FromAccountID)
	toID, _ := uuid.Parse(req.ToAccountID)
	idempotencyKey, _ := uuid.Parse(req.IdempotencyKey)
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
	if err := authorizeOwner(payload, fromAccount.Owner); err != nil {
		return err
	}

	result, err := s.store.TransferTx(ctx, store.TransferTxParams{
		FromAccountID:  fromID,
		ToAccountID:    toID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		IdempotencyKey: idempotencyKey,
		DailyLimit:     limit.Daily,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// listTransfers returns the transfer history for an account the caller owns,
// covering both sent and received transfers, newest first.
func (s *Server) listTransfers(c *echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid account id")
	}

	page, err := echo.QueryParamOr[int32](c, "page", 1)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid page")
	}
	size, err := echo.QueryParamOr[int32](c, "size", 10)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid size")
	}
	page = max(page, 1)
	if size < 1 {
		size = 10
	}
	size = min(size, 100)

	ctx := c.Request().Context()

	// Authorize against the account's owner before returning its history, the
	// same ownership check getAccount performs.
	account, err := s.store.GetAccount(ctx, id)
	if err != nil {
		return store.ClassifyError(err)
	}
	payload, err := authPayload(c)
	if err != nil {
		return err
	}
	if err := authorizeOwner(payload, account.Owner); err != nil {
		return err
	}

	// Compute the offset in int64 so a large page cannot silently overflow the
	// int32 offset column. A page past the addressable range holds no rows.
	offset := int64(page-1) * int64(size)
	if offset > math.MaxInt32 {
		return c.JSON(http.StatusOK, []sqlcdb.Transfer{})
	}

	transfers, err := s.store.ListTransfersByAccount(ctx, sqlcdb.ListTransfersByAccountParams{
		AccountID:  id,
		PageLimit:  size,
		PageOffset: int32(offset),
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusOK, transfers)
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
