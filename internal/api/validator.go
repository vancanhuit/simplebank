package api

import (
	"net/http"
	"strconv"

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
	v := validator.New(validator.WithRequiredStructEnabled())
	if err := v.RegisterValidation("maxbytes", validateMaxBytes); err != nil {
		panic(err)
	}
	return &customValidator{validate: v}
}

// bindValidate binds the request body into a fresh T and validates it,
// centralising the preamble every handler shares. It returns a 400 on a
// malformed body and the validator's own error on a failed rule.
func bindValidate[T any](c *echo.Context) (T, error) {
	var req T
	if err := c.Bind(&req); err != nil {
		return req, echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return req, err
	}
	return req, nil
}

// validateMaxBytes enforces a maximum length in bytes (not runes). The builtin
// max rule counts runes, which lets a multibyte string exceed a byte-oriented
// limit such as bcrypt's 72-byte cap.
func validateMaxBytes(fl validator.FieldLevel) bool {
	limit, err := strconv.Atoi(fl.Param())
	if err != nil {
		return false
	}
	return len(fl.Field().String()) <= limit
}
