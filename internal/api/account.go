package api

import (
	"math"
	"net/http"
	"uuid"

	"github.com/labstack/echo/v5"

	"github.com/vancanhuit/simplebank/internal/currency"
	store "github.com/vancanhuit/simplebank/internal/db"
	sqlcdb "github.com/vancanhuit/simplebank/internal/db/sqlc"
)

type createAccountRequest struct {
	Currency string `json:"currency" validate:"required"`
	// Balance is an optional opening deposit in minor units (e.g. cents). This
	// is a demo affordance for seeding funds; a real bank would fund accounts
	// through a payment rail, not a client-supplied opening balance.
	Balance int64 `json:"balance" validate:"min=0"`
}

func (s *Server) createAccount(c *echo.Context) error {
	req, err := bindValidate[createAccountRequest](c)
	if err != nil {
		return err
	}
	if !currency.IsSupported(req.Currency) {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported currency")
	}

	if req.Balance > currency.MaxSafeMinorUnits {
		return echo.NewHTTPError(
			http.StatusUnprocessableEntity,
			"opening balance exceeds the supported limit",
		)
	}

	if req.Balance > s.config.OpeningBalanceLimitFor(req.Currency) {
		return echo.NewHTTPError(
			http.StatusUnprocessableEntity,
			"opening balance exceeds the configured limit",
		)
	}

	payload, err := authPayload(c)
	if err != nil {
		return err
	}

	account, err := s.store.CreateAccountTx(c.Request().Context(), sqlcdb.CreateAccountParams{
		Owner:    payload.Username,
		Balance:  req.Balance,
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
	if err := authorizeOwner(payload, account.Owner); err != nil {
		return err
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
	page = max(page, 1)
	if size < 1 {
		size = 5
	}
	size = min(size, 100)

	// Compute the offset in int64 so a large page cannot silently overflow the
	// int32 offset column. A page past the addressable range holds no rows.
	offset := int64(page-1) * int64(size)
	if offset > math.MaxInt32 {
		return c.JSON(http.StatusOK, []sqlcdb.Account{})
	}
	//nolint:gosec // The upper bound is checked immediately above.
	offset32 := int32(offset)

	payload, err := authPayload(c)
	if err != nil {
		return err
	}

	accounts, err := s.store.ListAccounts(c.Request().Context(), sqlcdb.ListAccountsParams{
		Owner:  payload.Username,
		Limit:  size,
		Offset: offset32,
	})
	if err != nil {
		return store.ClassifyError(err)
	}
	return c.JSON(http.StatusOK, accounts)
}
