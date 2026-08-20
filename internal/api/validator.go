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
	if err := v.RegisterValidation("minbytes", validateMinBytes); err != nil {
		panic(err)
	}
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

// validateMinBytes and validateMaxBytes enforce byte-length bounds (not runes).
// The builtin min/max rules count runes, which lets a multibyte string slip
// past a byte-oriented policy such as bcrypt's 72-byte cap.
func validateMinBytes(fl validator.FieldLevel) bool {
	return validateBytes(fl, func(length, limit int) bool {
		return length >= limit
	})
}

func validateMaxBytes(fl validator.FieldLevel) bool {
	return validateBytes(fl, func(length, limit int) bool {
		return length <= limit
	})
}

func validateBytes(fl validator.FieldLevel, ok func(length, limit int) bool) bool {
	limit, err := strconv.Atoi(fl.Param())
	if err != nil {
		return false
	}
	return ok(len(fl.Field().String()), limit)
}
