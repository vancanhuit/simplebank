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
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 5
	}
	if size > 100 {
		size = 100
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
