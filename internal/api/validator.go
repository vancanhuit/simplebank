package api

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
)

type customValidator struct {
	validate *validator.Validate
}

func (cv *customValidator) Validate(i any) error {
	if err := cv.validate.Struct(i); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request payload")
	}
	return nil
}

func newValidator() *customValidator {
	return &customValidator{validate: validator.New()}
}
