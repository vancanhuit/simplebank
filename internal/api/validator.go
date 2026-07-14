package api

import (
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
)

type customValidator struct {
	validate *validator.Validate
}

func (cv *customValidator) Validate(i any) error {
	if err := cv.validate.Struct(i); err != nil {
		return echo.NewHTTPError(400, err.Error())
	}
	return nil
}

func newValidator() *customValidator {
	return &customValidator{validate: validator.New()}
}
